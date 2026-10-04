package proxy

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"io"
	"math/big"
	"net"
	"testing"
	"time"
)

func testTLS(t *testing.T) (*tls.Config, *tls.Config) {
	t.Helper()
	k, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "localhost"},
		DNSNames:              []string{"localhost"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	der, e := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &k.PublicKey, k)
	if e != nil {
		t.Fatal(e)
	}
	cert, _ := x509.ParseCertificate(der)
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	return &tls.Config{
			Certificates: []tls.Certificate{{Certificate: [][]byte{der}, PrivateKey: k}},
			NextProtos:   []string{"homeproxy/1"},
		}, &tls.Config{
			RootCAs:    pool,
			ServerName: "localhost",
			NextProtos: []string{"homeproxy/1"},
		}
}

func waitAgents(t *testing.T, s *server, n int) {
	t.Helper()
	until := time.Now().Add(3 * time.Second)
	for time.Now().Before(until) {
		s.mu.Lock()
		got := len(s.agents)
		s.mu.Unlock()
		if got == n {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("agent count timeout")
}

func socks(t *testing.T, addr string, cmd byte, target string) net.Conn {
	t.Helper()
	c, e := net.Dial("tcp", addr)
	if e != nil {
		t.Fatal(e)
	}
	c.SetDeadline(time.Now().Add(3 * time.Second))
	c.Write([]byte{5, 1, 0})
	b := make([]byte, 2)
	if _, e = io.ReadFull(c, b); e != nil || !bytes.Equal(b, []byte{5, 0}) {
		t.Fatalf("greeting %v %v", b, e)
	}
	a, e := encodeAddress(target)
	if e != nil {
		t.Fatal(e)
	}
	c.Write(append([]byte{5, cmd, 0}, a...))
	return c
}

func reply(t *testing.T, c net.Conn) string {
	t.Helper()
	b := make([]byte, 3)
	if _, e := io.ReadFull(c, b); e != nil {
		t.Fatal(e)
	}
	if b[1] != 0 {
		t.Fatalf("SOCKS error %v", b)
	}
	a, e := readAddress(c)
	if e != nil {
		t.Fatal(e)
	}
	return a
}

func TestUDPAssociation(t *testing.T) {
	st, ct := testTLS(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := newServer("secret", "priority")
	qa, _ := s.listenQUIC(ctx, "127.0.0.1:0", st)
	sa, _ := s.listenSOCKS(ctx, "127.0.0.1:0")
	go runAgent(ctx, qa, ct, "secret", "a", 1, true)
	waitAgents(t, s, 1)
	echo, _ := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	defer echo.Close()
	go func() {
		b := make([]byte, 2048)
		for {
			n, a, e := echo.ReadFromUDP(b)
			if e != nil {
				return
			}
			echo.WriteToUDP(b[:n], a)
		}
	}()
	u, _ := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	defer u.Close()
	c := socks(t, sa, 3, u.LocalAddr().String())
	relay, e := net.ResolveUDPAddr("udp", reply(t, c))
	if e != nil {
		t.Fatal(e)
	}
	a, _ := encodeAddress(echo.LocalAddr().String())
	packet := append(append([]byte{0, 0, 0}, a...), []byte("udp echo")...)
	u.WriteToUDP(packet, relay)
	u.SetReadDeadline(time.Now().Add(time.Second))
	b := make([]byte, 2048)
	n, _, e := u.ReadFromUDP(b)
	if e != nil || !bytes.Equal(packet, b[:n]) {
		t.Fatalf("UDP %v %q", e, b[:n])
	}
	wrong, _ := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	defer wrong.Close()
	wrong.WriteToUDP(packet, relay)
	wrong.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	if _, _, e = wrong.ReadFromUDP(b); e == nil {
		t.Fatal("wrong source accepted")
	}
	packet[2] = 1
	u.WriteToUDP(packet, relay)
	u.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	if _, _, e = u.ReadFromUDP(b); e == nil {
		t.Fatal("FRAG accepted")
	}
	packet[2] = 0
	u.WriteToUDP(append(packet, make([]byte, 1500)...), relay)
	u.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	if _, _, e = u.ReadFromUDP(b); e == nil {
		t.Fatal("oversize accepted")
	}
	c.Close()
	time.Sleep(50 * time.Millisecond)
	u.WriteToUDP(packet, relay)
	u.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	if _, _, e = u.ReadFromUDP(b); e == nil {
		t.Fatal("closed association accepted")
	}
}

func TestTwoAgentsUDPIsolationFailover(t *testing.T) {
	st, ct := testTLS(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := newServer("secret", "priority")
	qa, _ := s.listenQUIC(ctx, "127.0.0.1:0", st)
	sa, _ := s.listenSOCKS(ctx, "127.0.0.1:0")
	ac, stop := context.WithCancel(ctx)
	defer stop()
	go runAgent(ac, qa, ct, "secret", "a", 1, true)
	go runAgent(ctx, qa, ct, "secret", "b", 2, true)
	waitAgents(t, s, 2)
	echo, _ := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	defer echo.Close()
	go func() {
		b := make([]byte, 2048)
		for {
			n, a, e := echo.ReadFromUDP(b)
			if e != nil {
				return
			}
			echo.WriteToUDP(b[:n], a)
		}
	}()
	a, _ := encodeAddress(echo.LocalAddr().String())
	u1, _ := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	defer u1.Close()
	c1 := socks(t, sa, 3, u1.LocalAddr().String())
	defer c1.Close()
	r1, _ := net.ResolveUDPAddr("udp", reply(t, c1))
	s.selectAgent("b")
	u2, _ := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	defer u2.Close()
	c2 := socks(t, sa, 3, u2.LocalAddr().String())
	defer c2.Close()
	r2, _ := net.ResolveUDPAddr("udp", reply(t, c2))
	check := func(u *net.UDPConn, r *net.UDPAddr, msg byte) {
		t.Helper()
		p := append(append([]byte{0, 0, 0}, a...), msg)
		u.WriteToUDP(p, r)
		u.SetReadDeadline(time.Now().Add(time.Second))
		b := make([]byte, 2048)
		n, _, e := u.ReadFromUDP(b)
		if e != nil || !bytes.Equal(p, b[:n]) {
			t.Fatal("UDP isolation", e)
		}
	}
	check(u1, r1, 1)
	check(u2, r2, 2)
	stop()
	waitAgents(t, s, 1)
	c1.SetReadDeadline(time.Now().Add(time.Second))
	if _, e := c1.Read(make([]byte, 1)); e == nil {
		t.Fatal("dead UDP control alive")
	}
	check(u2, r2, 3)
	c3 := socks(t, sa, 3, u1.LocalAddr().String())
	defer c3.Close()
	r3, _ := net.ResolveUDPAddr("udp", reply(t, c3))
	check(u1, r3, 4)
}

func TestTwoAgentsPinnedFailover(t *testing.T) {
	st, ct := testTLS(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := newServer("secret", "priority")
	qa, _ := s.listenQUIC(ctx, "127.0.0.1:0", st)
	sa, _ := s.listenSOCKS(ctx, "127.0.0.1:0")
	a, stopA := context.WithCancel(ctx)
	defer stopA()
	go runAgent(a, qa, ct, "secret", "a", 1, true)
	go runAgent(ctx, qa, ct, "secret", "b", 2, true)
	waitAgents(t, s, 2)
	echo, _ := net.Listen("tcp", "127.0.0.1:0")
	defer echo.Close()
	go func() {
		for {
			c, e := echo.Accept()
			if e != nil {
				return
			}
			go func() { defer c.Close(); io.Copy(c, c) }()
		}
	}()
	ca := socks(t, sa, 1, echo.Addr().String())
	defer ca.Close()
	reply(t, ca)
	if s.choose().id != "a" {
		t.Fatal("priority")
	}
	if e := s.selectAgent("b"); e != nil {
		t.Fatal(e)
	}
	cb := socks(t, sa, 1, echo.Addr().String())
	defer cb.Close()
	reply(t, cb)
	for i, c := range []net.Conn{ca, cb} {
		msg := []byte{byte(i + 1)}
		c.Write(msg)
		b := make([]byte, 1)
		if _, e := io.ReadFull(c, b); e != nil || !bytes.Equal(b, msg) {
			t.Fatal("cross-session leakage", e)
		}
	}
	stopA()
	waitAgents(t, s, 1)
	ca.SetReadDeadline(time.Now().Add(time.Second))
	if _, e := ca.Read(make([]byte, 1)); e == nil {
		t.Fatal("dead agent TCP survived")
	}
	cb.Write([]byte("b"))
	b := make([]byte, 1)
	if _, e := io.ReadFull(cb, b); e != nil || b[0] != 'b' {
		t.Fatal("healthy session closed", e)
	}
	cc := socks(t, sa, 1, echo.Addr().String())
	defer cc.Close()
	reply(t, cc)
	cc.Write([]byte("c"))
	if _, e := io.ReadFull(cc, b); e != nil || b[0] != 'c' {
		t.Fatal("fallback failed", e)
	}
}

func TestAddressAndDestinationPolicy(t *testing.T) {
	for _, addr := range []string{"127.0.0.1:1234", "[::1]:1234", "localhost:1234"} {
		b, e := encodeAddress(addr)
		if e != nil {
			t.Fatal(e)
		}
		got, e := readAddress(bytes.NewReader(b))
		if e != nil || got != addr {
			t.Fatalf("address %s %s %v", addr, got, e)
		}
	}
	for _, addr := range []string{"127.0.0.1:80", "[::1]:80", "10.1.2.3:80", "169.254.1.1:80", "localhost:80"} {
		if _, e := safeTarget(context.Background(), addr, false); e == nil {
			t.Fatalf("private accepted: %s", addr)
		}
	}
	if _, e := safeTarget(context.Background(), "127.0.0.1:80", true); e != nil {
		t.Fatal(e)
	}
	if _, e := readAddress(bytes.NewReader([]byte{7})); e == nil {
		t.Fatal("invalid type")
	}
}

func TestUntrustedTLS(t *testing.T) {
	st, _ := testTLS(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := newServer("secret", "priority")
	qa, _ := s.listenQUIC(ctx, "127.0.0.1:0", st)
	err := runAgent(ctx, qa, &tls.Config{ServerName: "localhost", NextProtos: []string{"homeproxy/1"}}, "secret", "a", 1, true)
	if err == nil {
		t.Fatal("untrusted cert accepted")
	}
	waitAgents(t, s, 0)
}

func TestTLSAuthTCPHalfClose(t *testing.T) {
	st, ct := testTLS(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := newServer("secret", "priority")
	qa, e := s.listenQUIC(ctx, "127.0.0.1:0", st)
	if e != nil {
		t.Fatal(e)
	}
	sa, e := s.listenSOCKS(ctx, "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	bad, bc := context.WithCancel(ctx)
	defer bc()
	go runAgent(bad, qa, ct, "wrong", "bad", 0, true)
	time.Sleep(150 * time.Millisecond)
	waitAgents(t, s, 0)
	go runAgent(ctx, qa, ct, "secret", "a", 1, true)
	waitAgents(t, s, 1)
	echo, _ := net.Listen("tcp", "127.0.0.1:0")
	defer echo.Close()
	go func() {
		c, _ := echo.Accept()
		if c == nil {
			return
		}
		defer c.Close()
		data, _ := io.ReadAll(c)
		c.Write(data)
	}()
	c := socks(t, sa, 1, echo.Addr().String())
	defer c.Close()
	reply(t, c)
	c.Write([]byte("half-close"))
	c.(*net.TCPConn).CloseWrite()
	b, e := io.ReadAll(c)
	if e != nil || string(b) != "half-close" {
		t.Fatalf("echo %q %v", b, e)
	}
}
