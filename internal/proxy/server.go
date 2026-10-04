package proxy

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"net"
	"strings"
	"time"

	"github.com/quic-go/quic-go"
)

func generateSelfSignedCert() (tls.Certificate, error) {
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "homeproxy"},
		DNSNames:              []string{"localhost", "homeproxy"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(10 * 365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &k.PublicKey, k)
	if err != nil {
		return tls.Certificate{}, err
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: k}, nil
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
