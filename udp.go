package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/quic-go/quic-go"
)

// ponytail: fixed datagram ceiling; upgrade only with negotiated limits, not fragmentation.
const maxDatagram = 1100

var oversize atomic.Uint64

func sendDatagram(c *quic.Conn, id uint64, b []byte) {
	if len(b)+8 > maxDatagram {
		oversize.Add(1)
		log.Print("oversize UDP dropped")
		return
	}
	d := make([]byte, 8, len(b)+8)
	binary.BigEndian.PutUint64(d, id)
	d = append(d, b...)
	if e := c.SendDatagram(d); e != nil {
		oversize.Add(1)
		log.Print("QUIC datagram dropped")
	}
}
func parsePacket(b []byte) (string, []byte, error) {
	if len(b) < 4 || b[0] != 0 || b[1] != 0 || b[2] != 0 {
		return "", nil, io.ErrUnexpectedEOF
	}
	r := bytes.NewReader(b[3:])
	a, e := readAddress(r)
	return a, b[len(b)-r.Len():], e
}
func (s *server) receiveDatagrams(p *agentPeer) {
	for {
		b, e := p.conn.ReceiveDatagram(p.conn.Context())
		if e != nil {
			return
		}
		if len(b) < 8 || len(b) > maxDatagram {
			continue
		}
		id := binary.BigEndian.Uint64(b)
		if v, ok := p.udp.Load(id); ok {
			select {
			case v.(chan []byte) <- b[8:]:
			default:
			}
		}
	}
}
func (s *server) serveUDP(ctx context.Context, c net.Conn, p *agentPeer, addr string) {
	replied := false
	failure := byte(1)
	defer func() {
		if !replied {
			socksFailure(c, failure)
		}
	}()
	ac, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	host, port, e := net.SplitHostPort(addr)
	if e != nil {
		return
	}
	ips, e := net.DefaultResolver.LookupIPAddr(ac, host)
	if e != nil || len(ips) == 0 {
		return
	}
	source, e := net.ResolveUDPAddr("udp", net.JoinHostPort(ips[0].IP.String(), port))
	if e != nil {
		return
	}
	remote := c.RemoteAddr().(*net.TCPAddr)
	if !source.IP.IsUnspecified() && !source.IP.Equal(remote.IP) {
		failure = 2
		return
	}
	source.IP = remote.IP
	local := c.LocalAddr().(*net.TCPAddr)
	u, e := net.ListenUDP("udp", &net.UDPAddr{IP: local.IP})
	if e != nil {
		return
	}
	defer u.Close()
	q, e := p.conn.OpenStreamSync(ac)
	if e != nil {
		return
	}
	defer q.Close()
	defer q.CancelRead(0)
	deadline, _ := ac.Deadline()
	q.SetDeadline(deadline)
	if _, e = q.Write([]byte{3}); e != nil {
		return
	}
	b := []byte{1}
	if _, e = io.ReadFull(q, b); e != nil || b[0] != 0 {
		return
	}
	q.SetDeadline(time.Now().Add(time.Hour))
	id := uint64(q.StreamID())
	queue := make(chan []byte, 32)
	p.udp.Store(id, queue)
	defer p.udp.Delete(id)
	a, _ := encodeAddress(u.LocalAddr().String())
	replied = true
	if _, e = c.Write(append([]byte{5, 0, 0}, a...)); e != nil {
		return
	}
	c.SetDeadline(time.Now().Add(time.Hour))
	done := make(chan struct{})
	defer close(done)
	go func() { io.Copy(io.Discard, c); u.Close(); q.Close() }()
	go func() {
		select {
		case <-ctx.Done():
			c.Close()
			u.Close()
		case <-p.conn.Context().Done():
			c.Close()
			u.Close()
		case <-done:
		}
	}()
	var mu sync.Mutex
	go func() {
		for {
			select {
			case b := <-queue:
				if _, _, e := parsePacket(b); e != nil {
					continue
				}
				mu.Lock()
				dest := *source
				mu.Unlock()
				if dest.Port != 0 {
					u.WriteToUDP(b, &dest)
				}
			case <-done:
				return
			}
		}
	}()
	buf := make([]byte, 65535)
	for {
		u.SetReadDeadline(time.Now().Add(60 * time.Second))
		n, from, e := u.ReadFromUDP(buf)
		if e != nil {
			return
		}
		mu.Lock()
		valid := from.IP.Equal(source.IP) && (source.Port == 0 || source.Port == from.Port)
		if valid && source.Port == 0 {
			source.Port = from.Port
		}
		mu.Unlock()
		if !valid {
			continue
		}
		if _, _, e = parsePacket(buf[:n]); e != nil {
			continue
		}
		sendDatagram(p.conn, id, buf[:n])
	}
}

type udpKey struct {
	c  *quic.Conn
	id uint64
}

var agentQueues sync.Map

func agentDatagrams(c *quic.Conn) {
	for {
		b, e := c.ReceiveDatagram(c.Context())
		if e != nil {
			return
		}
		if len(b) < 8 || len(b) > maxDatagram {
			continue
		}
		if v, ok := agentQueues.Load(udpKey{c, binary.BigEndian.Uint64(b)}); ok {
			select {
			case v.(chan []byte) <- b[8:]:
			default:
			}
		}
	}
}
func agentUDP(c *quic.Conn, q *quic.Stream, allow bool) {
	defer q.Close()
	defer q.CancelRead(0)
	u, e := net.ListenUDP("udp", nil)
	if e != nil {
		return
	}
	defer u.Close()
	key := udpKey{c, uint64(q.StreamID())}
	queue := make(chan []byte, 32)
	agentQueues.Store(key, queue)
	defer agentQueues.Delete(key)
	q.Write([]byte{0})
	q.SetDeadline(time.Now().Add(time.Hour))
	done := make(chan struct{})
	defer close(done)
	go func() { io.Copy(io.Discard, q); u.Close() }()
	var mu sync.Mutex
	allowed := map[string]bool{}
	go func() {
		for {
			select {
			case b := <-queue:
				a, data, e := parsePacket(b)
				if e != nil {
					continue
				}
				ctx, cancel := context.WithTimeout(c.Context(), timeout)
				target, e := safeTarget(ctx, a, allow)
				cancel()
				if e != nil {
					continue
				}
				dst, e := net.ResolveUDPAddr("udp", target)
				if e != nil {
					continue
				}
				mu.Lock()
				ok := allowed[dst.String()] || len(allowed) < 32
				if ok {
					allowed[dst.String()] = true
				}
				mu.Unlock()
				if ok {
					u.WriteToUDP(data, dst)
				}
			case <-done:
				return
			}
		}
	}()
	b := make([]byte, 65535)
	for {
		u.SetReadDeadline(time.Now().Add(60 * time.Second))
		n, from, e := u.ReadFromUDP(b)
		if e != nil {
			return
		}
		mu.Lock()
		ok := allowed[from.String()]
		mu.Unlock()
		if !ok {
			continue
		}
		a, _ := encodeAddress(from.String())
		packet := append(append([]byte{0, 0, 0}, a...), b[:n]...)
		sendDatagram(c, key.id, packet)
	}
}
