package proxy

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
	"time"
)

func TestMobileAgentPinnedLifecycle(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	serverTLS, _ := testTLS(t)
	s := newServer("mobile-test-token-123", "priority")
	addr, err := s.listenQUIC(ctx, "127.0.0.1:0", serverTLS)
	if err != nil {
		t.Fatal(err)
	}
	pin := sha256.Sum256(serverTLS.Certificates[0].Certificate[0])
	a := NewMobileAgent()
	r := &statusRecorder{make(chan string, 16)}
	a.SetStatusListener(r)
	defer a.Stop()
	if err := a.Start(addr, "telefon", s.token, hex.EncodeToString(pin[:])); err != nil {
		t.Fatal(err)
	}
	waitAgents(t, s, 1)
	connected := false
	deadline := time.After(time.Second)
	for !connected {
		select {
		case status := <-r.updates:
			connected = status == "Bağlı"
		case <-deadline:
			t.Fatal("no authenticated connected callback")
		}
	}
	if err := a.Start(addr, "telefon", s.token, hex.EncodeToString(pin[:])); err == nil {
		t.Fatal("duplicate start accepted")
	}
	a.Stop()
	waitAgents(t, s, 0)
	if err := a.StartWithTLS(addr, "telefon", s.token, "", true); err != nil {
		t.Fatal(err)
	}
	waitAgents(t, s, 1)
	a.Stop()
	waitAgents(t, s, 0)
	if a.Status() != "Durduruldu" {
		t.Fatal(a.Status())
	}
	if err := a.Start(addr, "telefon", s.token, "invalid"); err == nil {
		t.Fatal("invalid pin accepted")
	}
}

func TestMobileAgentWrongPinRejected(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	serverTLS, _ := testTLS(t)
	s := newServer("mobile-test-token-123", "priority")
	addr, err := s.listenQUIC(ctx, "127.0.0.1:0", serverTLS)
	if err != nil {
		t.Fatal(err)
	}
	a := NewMobileAgent()
	r := &statusRecorder{make(chan string, 16)}
	a.SetStatusListener(r)
	defer a.Stop()
	// A valid but wrong fingerprint must fail before agent authentication.
	if err := a.Start(addr, "telefon", s.token, "0000000000000000000000000000000000000000000000000000000000000000"); err != nil {
		t.Fatal(err)
	}
	for a.Status() == "Bağlanıyor" && ctx.Err() == nil {
		time.Sleep(time.Millisecond)
	}
	if !strings.HasPrefix(a.Status(), "Bağlantı kesildi; ") {
		t.Fatal(a.Status())
	}
	for {
		select {
		case status := <-r.updates:
			if status == "Bağlı" {
				t.Fatal("wrong pin emitted connected callback")
			}
			if strings.HasPrefix(status, "Bağlantı kesildi;") {
				goto disconnected
			}
		case <-ctx.Done():
			t.Fatal("no reconnect callback")
		}
	}
disconnected:
	a.Stop()
	if s.choose() != nil {
		t.Fatal("untrusted agent registered")
	}
}
