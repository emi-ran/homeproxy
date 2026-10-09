package proxy

import (
	"bytes"
	"context"
	"net"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/quic-go/quic-go"
)

// Test-only UDP router: two sockets preserve direction and enforce a mutable
// UDP payload ceiling without host network changes or production MTU knobs.
func pathPair(t *testing.T, ceiling int64) (*quic.Conn, *quic.Conn, *atomic.Int64, *atomic.Uint64) {
	t.Helper()
	st, ct := testTLS(t)
	l, err := quic.ListenAddr("127.0.0.1:0", st, qc)
	if err != nil {
		t.Fatal(err)
	}
	front, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	back, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1)})
	if err != nil {
		t.Fatal(err)
	}
	var limit atomic.Int64
	limit.Store(ceiling)
	var drops atomic.Uint64
	var clientAddr atomic.Pointer[net.UDPAddr]
	done := make(chan struct{}, 2)
	go func() {
		defer func() { done <- struct{}{} }()
		b := make([]byte, 65536)
		for {
			n, a, e := front.ReadFromUDP(b)
			if e != nil {
				return
			}
			clientAddr.Store(a)
			if int64(n) > limit.Load() {
				drops.Add(1)
				continue
			}
			back.WriteTo(b[:n], l.Addr())
		}
	}()
	go func() {
		defer func() { done <- struct{}{} }()
		b := make([]byte, 65536)
		for {
			n, _, e := back.ReadFromUDP(b)
			if e != nil {
				return
			}
			if int64(n) > limit.Load() {
				drops.Add(1)
				continue
			}
			if a := clientAddr.Load(); a != nil {
				front.WriteToUDP(b[:n], a)
			}
		}
	}()
	t.Cleanup(func() { front.Close(); back.Close(); l.Close(); <-done; <-done })
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	c, err := quic.DialAddr(ctx, front.LocalAddr().String(), ct, qc)
	if err != nil {
		t.Logf("ceiling %d handshake unavailable: %v (dropped=%d)", ceiling, err, drops.Load())
		return nil, nil, &limit, &drops
	}
	s, err := l.Accept(ctx)
	if err != nil {
		c.CloseWithError(0, "test")
		t.Fatal(err)
	}
	t.Cleanup(func() { c.CloseWithError(0, "test"); s.CloseWithError(0, "test") })
	return c, s, &limit, &drops
}

func TestQUICLowPathAndShrinkDiagnostic(t *testing.T) {
	for _, ceiling := range []int64{1200, 1280, 1452} {
		t.Run(strconv.FormatInt(ceiling, 10), func(t *testing.T) {
			c, s, limit, drops := pathPair(t, ceiling)
			if c == nil {
				if ceiling != 1200 || drops.Load() == 0 {
					t.Fatal("unexpected handshake failure")
				}
				return
			}
			transfer := func(payload []byte) bool {
				for _, pair := range [][2]*quic.Conn{{c, s}, {s, c}} {
					if e := pair[0].SendDatagram(payload); e != nil {
						t.Logf("ceiling %d enqueue rejected: %v", limit.Load(), e)
						return false
					}
					ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
					b, e := pair[1].ReceiveDatagram(ctx)
					cancel()
					if e != nil {
						t.Logf("ceiling %d delivery unavailable: %v", limit.Load(), e)
						return false
					}
					if !bytes.Equal(b, payload) {
						t.Fatal("payload changed")
					}
				}
				return true
			}
			if !transfer([]byte{1}) {
				t.Fatal("small bidirectional datagram failed on initial path")
			}
			large := bytes.Repeat([]byte{0x5a}, 1410)
			transfer(large) // Early availability is diagnostic, not a portable invariant.
			for i := 0; i < 30; i++ {
				if !transfer([]byte{1}) {
					t.Fatal("small traffic failed")
				}
				time.Sleep(10 * time.Millisecond)
			}
			t.Logf("ceiling=%d warm near-MTU delivery=%v dropped packets=%d", ceiling, transfer(large), drops.Load())
			if ceiling == 1452 {
				limit.Store(1200)
				// Shrink can invalidate queued datagrams; no retry or synthetic MTU override.
				t.Logf("after shrink near-MTU delivery=%v", transfer(large))
				t.Logf("after shrink small delivery=%v dropped packets=%d", transfer([]byte{1}), drops.Load())
			}
		})
	}
}
