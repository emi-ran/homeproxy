package proxy

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"log"
	"net"
	"strconv"
	"sync"
	"time"

	"github.com/quic-go/quic-go"
)

// ponytail: fixed datagram ceiling; upgrade only with negotiated limits, not fragmentation.
// Must clear ENet HOST_DEFAULT_MTU (1392) + 8 tunnel id + 10 SOCKS5 UDP header = 1410,
// otherwise every near-MTU ENet packet is dropped and reliable UDP retransmits forever.
// 1452 = IPv4/1500 path UDP payload ceiling. IPv6-only <1280 paths still need Mori set_mtu.
const maxDatagram = 1452

func sendDatagram(c *quic.Conn, id uint64, b []byte, toAgent bool) {
	udpMetrics.send(c.SendDatagram, id, b, toAgent)
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
		udpMetrics.received(len(b), true)
		id := binary.BigEndian.Uint64(b)
		if v, ok := p.udp.Load(id); ok {
			udpMetrics.enqueue(v.(chan []byte), b[8:], true)
		}
	}
}

func isLocalInterfaceIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	ifaces, err := net.Interfaces()
	if err != nil {
		return false
	}
	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok {
				if ipnet.IP.Equal(ip) {
					return true
				}
			}
		}
	}
	return false
}

func getRelayIP(local, remote *net.TCPAddr) net.IP {
	ifaces, err := net.Interfaces()
	if err == nil {
		for _, iface := range ifaces {
			if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
				continue
			}
			addrs, err := iface.Addrs()
			if err != nil {
				continue
			}
			for _, addr := range addrs {
				if ipnet, ok := addr.(*net.IPNet); ok {
					if ip4 := ipnet.IP.To4(); ip4 != nil {
						if remote != nil && ipnet.Contains(remote.IP) {
							return ip4
						}
					}
				}
			}
		}
		for _, iface := range ifaces {
			if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
				continue
			}
			addrs, err := iface.Addrs()
			if err != nil {
				continue
			}
			for _, addr := range addrs {
				if ipnet, ok := addr.(*net.IPNet); ok {
					if ip4 := ipnet.IP.To4(); ip4 != nil {
						return ip4
					}
				}
			}
		}
	}
	if local != nil && local.IP != nil && !local.IP.IsUnspecified() && !local.IP.IsLoopback() {
		return local.IP
	}
	return net.IPv4zero
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
		log.Printf("serveUDP: invalid addr %q: %v", addr, e)
		return
	}
	var srcIP net.IP
	if ip := net.ParseIP(host); ip != nil {
		srcIP = ip
	} else if ips, err := net.DefaultResolver.LookupIPAddr(ac, host); err == nil && len(ips) > 0 {
		srcIP = ips[0].IP
	}
	srcPort, _ := strconv.Atoi(port)
	remote := c.RemoteAddr().(*net.TCPAddr)
	local := c.LocalAddr().(*net.TCPAddr)

	if srcIP != nil && !srcIP.IsUnspecified() && !srcIP.Equal(remote.IP) {
		failure = 2
		return
	}

	var u *net.UDPConn
	if local != nil && local.IP != nil && !local.IP.IsUnspecified() && isLocalInterfaceIP(local.IP) {
		u, e = net.ListenUDP("udp", &net.UDPAddr{IP: local.IP})
		if e != nil {
			log.Printf("serveUDP: ListenUDP on %v failed: %v, falling back to 0.0.0.0", local.IP, e)
		}
	}
	if u == nil {
		u, e = net.ListenUDP("udp", nil)
		if e != nil {
			log.Printf("serveUDP: ListenUDP fallback failed: %v", e)
			return
		}
	}
	defer u.Close()

	q, e := p.conn.OpenStreamSync(ac)
	if e != nil {
		log.Printf("serveUDP: OpenStreamSync failed: %v", e)
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
	q.SetDeadline(time.Now().Add(sessionLifetime(ctx)))
	id := uint64(q.StreamID())
	queue := make(chan []byte, 32)
	p.udp.Store(id, queue)
	defer p.udp.Delete(id)

	relayPort := u.LocalAddr().(*net.UDPAddr).Port
	var advertiseIP net.IP
	if local != nil && local.IP != nil && !local.IP.IsUnspecified() && isLocalInterfaceIP(local.IP) {
		advertiseIP = local.IP
	} else {
		advertiseIP = getRelayIP(local, remote)
	}
	bndAddr := net.JoinHostPort(advertiseIP.String(), strconv.Itoa(relayPort))
	a, _ := encodeAddress(bndAddr)
	replied = true
	if _, e = c.Write(append([]byte{5, 0, 0}, a...)); e != nil {
		return
	}
	log.Printf("serveUDP: association established for client %v, relay listening at %s", remote, bndAddr)

	c.SetDeadline(time.Now().Add(sessionLifetime(ctx)))
	defer c.Close()
	done := make(chan struct{})
	defer close(done)
	go func() { io.Copy(io.Discard, c); u.Close(); q.Close() }()
	go func() { io.Copy(io.Discard, q); c.Close(); u.Close() }()
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

	var source *net.UDPAddr
	if srcPort != 0 && srcIP != nil && !srcIP.IsUnspecified() {
		source = &net.UDPAddr{IP: srcIP, Port: srcPort}
	} else {
		source = &net.UDPAddr{IP: remote.IP, Port: srcPort}
	}

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
			if ne, ok := e.(net.Error); ok && ne.Timeout() {
				// Downlink silence must not tear the association (client re-associate churn);
				// lifetime stays bounded by the 1h conn/stream deadlines.
				u.SetReadDeadline(time.Now().Add(60 * time.Second))
				continue
			}
			return
		}
		if _, _, e = parsePacket(buf[:n]); e != nil {
			continue
		}
		mu.Lock()
		if source.Port == 0 && from.IP.Equal(source.IP) {
			source.Port = from.Port
		}
		valid := from.IP.Equal(source.IP) && from.Port == source.Port
		mu.Unlock()
		if !valid {
			udpMetrics.sourceRejects.Add(1)
			continue
		}
		sendDatagram(p.conn, id, buf[:n], true)
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
		udpMetrics.received(len(b), false)
		if v, ok := agentQueues.Load(udpKey{c, binary.BigEndian.Uint64(b)}); ok {
			udpMetrics.enqueue(v.(chan []byte), b[8:], false)
		}
	}
}

func agentUDP(c *quic.Conn, q *quic.Stream, allow bool) {
	agentUDPTimeouts(c, q, allow, 60*time.Second, time.Hour)
}

func agentUDPTimeouts(c *quic.Conn, q *quic.Stream, allow bool, idle, lifetime time.Duration) {
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
	u.SetReadDeadline(time.Now().Add(idle))
	q.Write([]byte{0})
	q.SetDeadline(time.Now().Add(lifetime))
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
				dst, e := resolveUDPTarget(ctx, a, allow, net.DefaultResolver.LookupIPAddr)
				cancel()
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
					if _, e := u.WriteToUDP(data, dst); e == nil {
						u.SetReadDeadline(time.Now().Add(idle))
					}
				}
			case <-done:
				return
			}
		}
	}()
	b := make([]byte, 65535)
	for {
		n, from, e := u.ReadFromUDP(b)
		if e != nil {
			if ne, ok := e.(net.Error); ok && ne.Timeout() {
				// Keep the association alive through downlink silence; the stream
				// lifetime deadline (or a real socket error) still ends the loop.
				u.SetReadDeadline(time.Now().Add(idle))
				continue
			}
			return
		}
		mu.Lock()
		ok := allowed[from.String()]
		mu.Unlock()
		if !ok {
			continue
		}
		u.SetReadDeadline(time.Now().Add(idle))
		a, _ := encodeAddress(from.String())
		packet := append(append([]byte{0, 0, 0}, a...), b[:n]...)
		sendDatagram(c, key.id, packet, false)
	}
}
