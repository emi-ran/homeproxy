package proxy

import (
	"context"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

func TestConfiguredServerLifetimeClosesTCP(t *testing.T) {
	st, ct := testTLS(t)
	ctx, cancel := context.WithCancel(context.WithValue(context.Background(), lifetimeKey{}, 80*time.Millisecond))
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
	// Leave the agent at its default cap so this verifies server ownership.
	go runAgent(context.WithValue(ctx, lifetimeKey{}, defaultSessionLifetime), qa, ct, "secret", "a", 1, true)
	waitAgents(t, s, 1)
	echo, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer echo.Close()
	go func() {
		c, e := echo.Accept()
		if e == nil {
			defer c.Close()
			io.Copy(c, c)
		}
	}()
	c := socks(t, sa, 1, echo.Addr().String())
	defer c.Close()
	reply(t, c)
	c.SetDeadline(time.Now().Add(time.Second))
	if _, err := c.Write([]byte("x")); err != nil {
		t.Fatal(err)
	}
	b := make([]byte, 1)
	if _, err := io.ReadFull(c, b); err != nil {
		t.Fatal(err)
	}
	c.SetReadDeadline(time.Now().Add(600 * time.Millisecond))
	_, err = c.Read(b)
	if err == nil {
		t.Fatal("session survived hard deadline")
	}
	if e, ok := err.(net.Error); ok && e.Timeout() {
		t.Fatal("configured hard lifetime not enforced")
	}
}

func TestConfiguredUDPLifetimeClosesControl(t *testing.T) {
	for _, side := range []string{"server", "agent"} {
		t.Run(side, func(t *testing.T) {
			st, ct := testTLS(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			serverCtx, agentCtx := ctx, ctx
			if side == "server" {
				serverCtx = context.WithValue(ctx, lifetimeKey{}, 80*time.Millisecond)
			} else {
				agentCtx = context.WithValue(ctx, lifetimeKey{}, 80*time.Millisecond)
			}
			s := newServer("secret", "priority")
			qa, err := s.listenQUIC(serverCtx, "127.0.0.1:0", st)
			if err != nil {
				t.Fatal(err)
			}
			sa, err := s.listenSOCKS(serverCtx, "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			go runAgent(agentCtx, qa, ct, "secret", "a", 1, true)
			waitAgents(t, s, 1)
			c := socks(t, sa, 3, "0.0.0.0:0")
			defer c.Close()
			reply(t, c)
			c.SetReadDeadline(time.Now().Add(600 * time.Millisecond))
			_, err = c.Read(make([]byte, 1))
			if err == nil {
				t.Fatal("control survived hard lifetime")
			}
			if e, ok := err.(net.Error); ok && e.Timeout() {
				t.Fatal("UDP configured lifetime not enforced")
			}
		})
	}
}

func TestMobileLifetimeValidationBeforeStart(t *testing.T) {
	for _, seconds := range []int64{0, -1, 86401, 9223372036854775807} {
		a := NewMobileAgent()
		err := a.StartWithTLSAndLifetime("127.0.0.1:1", "test", "0123456789abcdef", "", true, seconds)
		if err == nil || !strings.Contains(err.Error(), "session lifetime") {
			a.Stop()
			t.Fatalf("%d: %v", seconds, err)
		}
		if a.Status() != "Durduruldu" {
			t.Fatal("invalid lifetime changed state")
		}
	}
}

func TestCLILifetimeRejectsUnboundedBeforeCredentials(t *testing.T) {
	for _, value := range []string{"0", "-1s", "25h", "invalid"} {
		err := RunCLI([]string{"server", "-session-lifetime", value})
		if err == nil || !strings.Contains(err.Error(), "session lifetime") {
			t.Fatalf("%s: expected bounded session lifetime rejection, got %v", value, err)
		}
	}
}
