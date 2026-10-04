package main

import (
	"context"
	"crypto/subtle"
	"crypto/tls"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/quic-go/quic-go"
)

const timeout = 10 * time.Second
const maxSessions = 128

var errAddressType = errors.New("address type")

func socksFailure(c net.Conn, code byte) {
	c.SetWriteDeadline(time.Now().Add(timeout))
	c.Write([]byte{5, code, 0, 1, 0, 0, 0, 0, 0, 0})
}

var qc = &quic.Config{EnableDatagrams: true, MaxIdleTimeout: 20 * time.Second, KeepAlivePeriod: 5 * time.Second, MaxIncomingStreams: 128, MaxIncomingUniStreams: -1}

type hello struct {
	Token, ID string
	Priority  int
}

func encodeAddress(addr string) ([]byte, error) {
	h, p, e := net.SplitHostPort(addr)
	if e != nil {
		return nil, e
	}
	port, e := strconv.Atoi(p)
	if e != nil || port < 0 || port > 65535 {
		return nil, errors.New("port")
	}
	var b []byte
	if ip := net.ParseIP(h); ip != nil {
		if v := ip.To4(); v != nil {
			b = append([]byte{1}, v...)
		} else {
			b = append([]byte{4}, ip.To16()...)
		}
	} else {
		if len(h) == 0 || len(h) > 255 {
			return nil, errors.New("host")
		}
		b = append([]byte{3, byte(len(h))}, []byte(h)...)
	}
	return append(b, byte(port>>8), byte(port)), nil
}
func readAddress(r io.Reader) (string, error) {
	b := make([]byte, 1)
	if _, e := io.ReadFull(r, b); e != nil {
		return "", e
	}
	n := 0
	domain := false
	switch b[0] {
	case 1:
		n = 4
	case 4:
		n = 16
	case 3:
		if _, e := io.ReadFull(r, b); e != nil {
			return "", e
		}
		n = int(b[0])
		domain = true
		if n == 0 {
			return "", errors.New("host")
		}
	default:
		return "", errAddressType
	}
	b = make([]byte, n+2)
	if _, e := io.ReadFull(r, b); e != nil {
		return "", e
	}
	h := string(b[:n])
	if !domain {
		h = net.IP(b[:n]).String()
	}
	return net.JoinHostPort(h, strconv.Itoa(int(binary.BigEndian.Uint16(b[n:])))), nil
}
func safeTarget(ctx context.Context, addr string, allow bool) (string, error) {
	h, p, e := net.SplitHostPort(addr)
	if e != nil || p == "0" {
		return "", errors.New("target")
	}
	ips, e := net.DefaultResolver.LookupIPAddr(ctx, h)
	if e != nil {
		return "", e
	}
	if len(ips) == 0 {
		return "", errors.New("dns")
	}
	for _, v := range ips {
		ip := v.IP
		if !allow && (ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() || !ip.IsGlobalUnicast()) {
			return "", errors.New("non-public destination blocked")
		}
	}
	return net.JoinHostPort(ips[0].IP.String(), p), nil
}
func bridge(c net.Conn, q *quic.Stream) {
	// ponytail: one-hour hard lifetime, not sliding idle timeout.
	c.SetDeadline(time.Now().Add(time.Hour))
	q.SetDeadline(time.Now().Add(time.Hour))
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); io.Copy(q, c); q.Close() }()
	go func() {
		defer wg.Done()
		io.Copy(c, q)
		if t, ok := c.(*net.TCPConn); ok {
			t.CloseWrite()
		}
	}()
	wg.Wait()
	q.CancelRead(0)
	c.Close()
}
func (s *server) listenQUIC(ctx context.Context, addr string, t *tls.Config) (string, error) {
	l, e := quic.ListenAddr(addr, t, qc)
	if e != nil {
		return "", e
	}
	go func() {
		<-ctx.Done()
		l.Close()
		s.mu.Lock()
		for _, p := range s.agents {
			p.conn.CloseWithError(0, "shutdown")
		}
		s.mu.Unlock()
	}()
	go func() {
		slots := make(chan struct{}, 4)
		for {
			c, e := l.Accept(ctx)
			if e != nil {
				return
			}
			select {
			case slots <- struct{}{}:
				go func() { defer func() { <-slots }(); s.register(ctx, c) }()
			default:
				c.CloseWithError(1, "capacity")
			}
		}
	}()
	return l.Addr().String(), nil
}
func (s *server) register(ctx context.Context, c *quic.Conn) {
	accepted := false
	defer func() {
		if !accepted {
			c.CloseWithError(1, "authentication rejected")
		}
	}()
	ac, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	q, e := c.AcceptStream(ac)
	if e != nil {
		return
	}
	q.SetDeadline(time.Now().Add(timeout))
	var h hello
	if e = json.NewDecoder(io.LimitReader(q, 1024)).Decode(&h); e != nil || len(h.ID) == 0 || len(h.ID) > 64 || strings.ContainsAny(h.ID, "\r\n") || subtle.ConstantTimeCompare([]byte(h.Token), []byte(s.token)) != 1 {
		return
	}
	s.mu.Lock()
	if len(s.agents) >= 2 || s.agents[h.ID] != nil {
		s.mu.Unlock()
		return
	}
	p := &agentPeer{id: h.ID, priority: h.Priority, conn: c}
	s.agents[h.ID] = p
	s.mu.Unlock()
	accepted = true
	q.Write([]byte{0})
	q.Close()
	q.CancelRead(0)
	go s.receiveDatagrams(p)
	<-c.Context().Done()
	s.mu.Lock()
	if s.agents[h.ID] == p {
		delete(s.agents, h.ID)
	}
	s.mu.Unlock()
}
func runAgent(ctx context.Context, addr string, t *tls.Config, token, id string, priority int, allow bool) error {
	c, e := quic.DialAddr(ctx, addr, t, qc)
	if e != nil {
		return e
	}
	defer c.CloseWithError(0, "agent stopped")
	go func() {
		select {
		case <-ctx.Done():
			c.CloseWithError(0, "shutdown")
		case <-c.Context().Done():
		}
	}()
	q, e := c.OpenStreamSync(ctx)
	if e != nil {
		return e
	}
	q.SetDeadline(time.Now().Add(timeout))
	if e = json.NewEncoder(q).Encode(hello{token, id, priority}); e != nil {
		return e
	}
	b := make([]byte, 1)
	if _, e = io.ReadFull(q, b); e != nil {
		return e
	}
	q.Close()
	q.CancelRead(0)
	go agentDatagrams(c)
	for {
		q, e := c.AcceptStream(ctx)
		if e != nil {
			return e
		}
		go agentSession(c, q, allow)
	}
}
func agentSession(c *quic.Conn, q *quic.Stream, allow bool) {
	q.SetDeadline(time.Now().Add(timeout))
	b := make([]byte, 1)
	if _, e := io.ReadFull(q, b); e != nil {
		q.CancelRead(1)
		q.Close()
		return
	}
	if b[0] == 3 {
		agentUDP(c, q, allow)
		return
	}
	if b[0] != 1 {
		q.Close()
		q.CancelRead(1)
		return
	}
	addr, e := readAddress(q)
	if e == nil {
		ctx, cancel := context.WithTimeout(c.Context(), timeout)
		defer cancel()
		addr, e = safeTarget(ctx, addr, allow)
	}
	var dst net.Conn
	if e == nil {
		dst, e = net.DialTimeout("tcp", addr, timeout)
	}
	if e != nil {
		q.Write([]byte{1})
		q.Close()
		q.CancelRead(1)
		return
	}
	a, e := encodeAddress(dst.LocalAddr().String())
	if e != nil {
		dst.Close()
		q.Close()
		q.CancelRead(1)
		return
	}
	if _, e = q.Write(append([]byte{0}, a...)); e != nil {
		dst.Close()
		q.Close()
		q.CancelRead(1)
		return
	}
	q.SetDeadline(time.Time{})
	done := make(chan struct{})
	go func() {
		select {
		case <-c.Context().Done():
			dst.Close()
		case <-done:
		}
	}()
	bridge(dst, q)
	close(done)
}
func (s *server) listenSOCKS(ctx context.Context, addr string) (string, error) {
	l, e := net.Listen("tcp", addr)
	if e != nil {
		return "", e
	}
	go func() { <-ctx.Done(); l.Close() }()
	go func() {
		slots := make(chan struct{}, maxSessions)
		for {
			c, e := l.Accept()
			if e != nil {
				return
			}
			select {
			case slots <- struct{}{}:
				go func() { defer func() { <-slots }(); s.handleSOCKS(ctx, c) }()
			default:
				c.Close()
			}
		}
	}()
	return l.Addr().String(), nil
}
func (s *server) handleSOCKS(ctx context.Context, c net.Conn) {
	defer c.Close()
	c.SetDeadline(time.Now().Add(timeout))
	b := make([]byte, 2)
	if _, e := io.ReadFull(c, b); e != nil || b[0] != 5 || b[1] == 0 {
		return
	}
	methods := make([]byte, int(b[1]))
	if _, e := io.ReadFull(c, methods); e != nil {
		return
	}
	ok := false
	for _, m := range methods {
		ok = ok || m == 0
	}
	if !ok {
		c.Write([]byte{5, 255})
		return
	}
	c.Write([]byte{5, 0})
	b = make([]byte, 3)
	if _, e := io.ReadFull(c, b); e != nil || b[0] != 5 || b[2] != 0 {
		return
	}
	addr, e := readAddress(c)
	if e != nil {
		if errors.Is(e, errAddressType) {
			socksFailure(c, 8)
		}
		return
	}
	p := s.choose()
	if p == nil {
		c.Write([]byte{5, 1, 0, 1, 0, 0, 0, 0, 0, 0})
		return
	}
	if b[1] == 3 {
		s.serveUDP(ctx, c, p, addr)
		return
	}
	if b[1] != 1 {
		c.Write([]byte{5, 7, 0, 1, 0, 0, 0, 0, 0, 0})
		return
	}
	replied := false
	failure := byte(1)
	defer func() {
		if !replied {
			socksFailure(c, failure)
		}
	}()
	oc, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	q, e := p.conn.OpenStreamSync(oc)
	if e != nil {
		return
	}
	defer q.CancelRead(0)
	defer q.Close()
	deadline, _ := oc.Deadline()
	q.SetDeadline(deadline)
	a, _ := encodeAddress(addr)
	if _, e = q.Write(append([]byte{1}, a...)); e != nil {
		return
	}
	status := []byte{1}
	if _, e = io.ReadFull(q, status); e != nil {
		return
	}
	if status[0] != 0 {
		failure = 2
		return
	}
	bound, e := readAddress(q)
	if e != nil {
		return
	}
	a, e = encodeAddress(bound)
	if e != nil {
		return
	}
	replied = true
	if _, e = c.Write(append([]byte{5, 0, 0}, a...)); e != nil {
		return
	}
	q.SetDeadline(time.Time{})
	c.SetDeadline(time.Time{})
	done := make(chan struct{})
	go func() {
		select {
		case <-p.conn.Context().Done():
			c.Close()
		case <-ctx.Done():
			c.Close()
		case <-done:
		}
	}()
	bridge(c, q)
	close(done)
}
