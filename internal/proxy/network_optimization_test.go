package proxy

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestRetryResetRequiresAuthenticatedDuration(t *testing.T) {
	now := time.Unix(100, 0)
	r := reconnectState{fails: 4}
	r.next(now.Add(time.Minute))
	if r.fails != 5 {
		t.Fatalf("unauthenticated attempt reset: %d", r.fails)
	}
	r.authenticated = now.Add(59 * time.Second)
	r.next(now.Add(time.Minute))
	if r.fails != 6 {
		t.Fatalf("brief authentication reset: %d", r.fails)
	}
	r.authenticated = now
	r.next(now.Add(30 * time.Second))
	if r.fails != 1 {
		t.Fatalf("healthy authentication did not reset: %d", r.fails)
	}
	if !r.authenticated.IsZero() {
		t.Fatal("authentication leaked into next attempt")
	}
}

func TestUDPTargetNumericBypassesResolver(t *testing.T) {
	calls := 0
	lookup := func(context.Context, string) ([]net.IPAddr, error) { calls++; return nil, nil }
	dst, err := resolveUDPTarget(context.Background(), "8.8.8.8:53", false, lookup)
	if err != nil || dst.String() != "8.8.8.8:53" || calls != 0 {
		t.Fatalf("dst=%v err=%v calls=%d", dst, err, calls)
	}
	for _, addr := range []string{"100.100.100.200:53", "[::ffff:127.0.0.1]:53", "8.8.8.8:0"} {
		if _, err := resolveUDPTarget(context.Background(), addr, false, lookup); err == nil {
			t.Fatalf("accepted %s", addr)
		}
	}
}

func TestUDPTargetRevalidatesAllDNSAnswers(t *testing.T) {
	calls := 0
	lookup := func(context.Context, string) ([]net.IPAddr, error) {
		calls++
		if calls == 1 {
			return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
		}
		return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}, {IP: net.ParseIP("100.100.100.200")}}, nil
	}
	if _, err := resolveUDPTarget(context.Background(), "example.test:53", false, lookup); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveUDPTarget(context.Background(), "example.test:53", false, lookup); err == nil {
		t.Fatal("stale policy or unchecked secondary answer")
	}
	if calls != 2 {
		t.Fatalf("calls=%d", calls)
	}
}

func TestUDPForeignValidPacketDoesNotPinPort(t *testing.T) {
	st, ct := testTLS(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := newServer("secret", "priority")
	qa, err := s.listenQUIC(ctx, "127.0.0.1:0", st)
	if err != nil {
		t.Fatal(err)
	}
	sa, err := s.listenSOCKS(ctx, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	go runAgent(ctx, qa, ct, "secret", "a", 1, true)
	waitAgents(t, s, 1)
	control := socks(t, sa, 3, "0.0.0.0:0")
	defer control.Close()
	relay, err := net.ResolveUDPAddr("udp", reply(t, control))
	if err != nil {
		t.Fatal(err)
	}
	bad, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.2")})
	if err != nil {
		t.Fatal(err)
	}
	defer bad.Close()
	good, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatal(err)
	}
	defer good.Close()
	echo, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatal(err)
	}
	defer echo.Close()
	go func() {
		b := make([]byte, 128)
		n, from, err := echo.ReadFromUDP(b)
		if err == nil {
			echo.WriteToUDP(b[:n], from)
		}
	}()
	a, _ := encodeAddress(echo.LocalAddr().String())
	packet := append(append([]byte{0, 0, 0}, a...), 1)
	rejected := func() uint64 { return SnapshotUDPStats().SourceRejects }
	before := rejected()
	if _, err := bad.WriteToUDP(packet, relay); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for rejected() == before {
		if time.Now().After(deadline) {
			t.Fatal("foreign packet was not processed")
		}
		time.Sleep(time.Millisecond)
	}
	if _, err := good.WriteToUDP(packet, relay); err != nil {
		t.Fatal(err)
	}
	good.SetReadDeadline(time.Now().Add(time.Second))
	if _, _, err := good.ReadFromUDP(make([]byte, 128)); err != nil {
		t.Fatalf("foreign valid packet pinned source: %v", err)
	}
}
