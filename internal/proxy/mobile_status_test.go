package proxy

import (
	"strings"
	"testing"
	"time"
)

type statusRecorder struct{ updates chan string }

func (r *statusRecorder) OnStatus(status string) { r.updates <- status }

func TestMobileStatusCallbackAndStopConnecting(t *testing.T) {
	a := NewMobileAgent()
	r := &statusRecorder{make(chan string, 16)}
	a.SetStatusListener(r)
	if got := <-r.updates; got != "Durduruldu" {
		t.Fatal(got)
	}
	// Unreachable localhost port: start returns before dialing, stop must cancel it.
	if err := a.StartWithTLS("127.0.0.1:1", "callback", "mobile-test-token-123", "", true); err != nil {
		t.Fatal(err)
	}
	defer a.Stop()
	select {
	case got := <-r.updates:
		if got != "Bağlanıyor" {
			t.Fatal(got)
		}
	case <-time.After(time.Second):
		t.Fatal("no connecting callback")
	}
	done := make(chan struct{})
	go func() { a.Stop(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("stop blocked while connecting")
	}
	if a.Status() != "Durduruldu" {
		t.Fatal(a.Status())
	}
	for {
		select {
		case got := <-r.updates:
			if got == "Durduruldu" {
				return
			}
			if got != "Bağlanıyor" && !strings.HasPrefix(got, "Bağlantı kesildi;") {
				t.Fatal(got)
			}
		case <-time.After(time.Second):
			t.Fatal("no stopped callback")
		}
	}
}
