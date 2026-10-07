package proxy

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

// MobileAgent owns one cancellable tunnel independently of the UI lifecycle.
type MobileAgent struct {
	mu       sync.Mutex
	cancel   context.CancelFunc
	done     chan struct{}
	status   string
	listener StatusListener
}

// StatusListener receives tunnel transitions; callbacks must not block.
type StatusListener interface{ OnStatus(string) }

func (a *MobileAgent) SetStatusListener(listener StatusListener) {
	a.mu.Lock()
	a.listener = listener
	status := a.status
	a.mu.Unlock()
	if listener != nil {
		listener.OnStatus(status)
	}
}

func NewMobileAgent() *MobileAgent { return &MobileAgent{status: "Durduruldu"} }

// Start authenticates the server with an independently obtained SHA-256 leaf certificate pin.
func (a *MobileAgent) Start(address, id, token, fingerprint string) error {
	return a.StartWithTLS(address, id, token, fingerprint, false)
}

// StartWithTLS permits unverified TLS only when explicitly requested by the user.
func (a *MobileAgent) StartWithTLS(address, id, token, fingerprint string, insecure bool) error {
	host, port, err := net.SplitHostPort(address)
	n, portErr := strconv.Atoi(port)
	if err != nil || host == "" || portErr != nil || n < 1 || n > 65535 {
		return errors.New("Sunucu adresi host:port biçiminde olmalı")
	}
	if len(id) == 0 || len(id) > 64 || strings.ContainsAny(id, "\r\n") {
		return errors.New("Agent ID 1–64 bayt olmalı")
	}
	if len(token) < 16 {
		return errors.New("Token en az 16 bayt olmalı")
	}
	pin, err := hex.DecodeString(strings.ReplaceAll(strings.TrimSpace(fingerprint), ":", ""))
	if !insecure && (err != nil || len(pin) != sha256.Size) {
		return errors.New("Sertifika SHA-256 parmak izi 64 hex karakter olmalı")
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cancel != nil {
		return errors.New("Agent zaten çalışıyor")
	}
	unlock, ok := acquireInstanceLock("Local\\HomeProxyAgent_" + id)
	if !ok {
		return errors.New("agent ID already running or instance lock unavailable")
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.cancel, a.done, a.status = cancel, make(chan struct{}), "Bağlanıyor"
	done := a.done
	t := &tls.Config{MinVersion: tls.VersionTLS13, NextProtos: []string{"homeproxy/1"}, InsecureSkipVerify: true,
		// Certificate pin replaces PKI verification; never accept an unpinned peer.
		VerifyConnection: func(c tls.ConnectionState) error {
			if insecure {
				return nil
			}
			if len(c.PeerCertificates) == 0 {
				return errors.New("Sunucu sertifikası yok")
			}
			h := sha256.Sum256(c.PeerCertificates[0].Raw)
			if subtle.ConstantTimeCompare(h[:], pin) != 1 {
				return errors.New("Sunucu sertifikası eşleşmiyor")
			}
			return nil
		},
	}
	go func() {
		defer unlock()
		defer close(done)
		fails := 0
		for ctx.Err() == nil {
			a.setStatus("Bağlanıyor")
			start := time.Now()
			err := runAgentStatus(ctx, address, t, token, id, 100, false, func() { a.setStatus("Bağlı") })
			if ctx.Err() != nil {
				break
			}
			if time.Since(start) > 30*time.Second {
				fails = 0 // a healthy run resets the backoff
			}
			d := reconnectDelay(fails)
			fails++
			if err != nil {
				a.setStatus(fmt.Sprintf("Bağlantı kesildi; %v sonra yeniden denenecek", d.Round(time.Second)))
			}
			select {
			case <-ctx.Done():
			case <-time.After(d):
			}
		}
		a.setStatus("Durduruldu")
	}()
	return nil
}

func (a *MobileAgent) setStatus(s string) {
	a.mu.Lock()
	a.status = s
	listener := a.listener
	a.mu.Unlock()
	if listener != nil {
		listener.OnStatus(s)
	}
}
func (a *MobileAgent) Status() string { a.mu.Lock(); defer a.mu.Unlock(); return a.status }
func (a *MobileAgent) Stop() {
	a.mu.Lock()
	cancel, done := a.cancel, a.done
	a.mu.Unlock()
	if cancel == nil {
		return
	}
	cancel()
	<-done
	a.mu.Lock()
	if a.done == done {
		a.cancel = nil
	}
	a.mu.Unlock()
}
