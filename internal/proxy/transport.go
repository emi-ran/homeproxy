package proxy

import (
	"io"
	"math/rand"
	"net"
	"sync"
	"time"

	"github.com/quic-go/quic-go"
)

const timeout = 10 * time.Second
const maxSessions = 128

var qc = &quic.Config{
	EnableDatagrams: true,
	// 5s keepalives keep the phone radio awake with zero traffic (heat + battery).
	// 30s > radio idle window, and idle must outlive one keepalive period.
	MaxIdleTimeout:  90 * time.Second,
	KeepAlivePeriod: 30 * time.Second,
	MaxIncomingStreams:    128,
	MaxIncomingUniStreams: -1,
}

type hello struct {
	Token, ID string
	Priority  int
}

// reconnectDelay: jittered exponential backoff, 5s base capped at 60s.
// A fixed 3s retry re-runs a full TLS 1.3 handshake while the network is down,
// which is the main self-inflicted heat source on the phone.
func reconnectDelay(fails int) time.Duration {
	d := 5 * time.Second
	for i := 0; i < fails && d < time.Minute; i++ {
		d *= 2
	}
	if d > time.Minute {
		d = time.Minute
	}
	return d/2 + time.Duration(rand.Int63n(int64(d/2)+1)) // jitter in [d/2, d]
}

func socksFailure(c net.Conn, code byte) {
	c.SetWriteDeadline(time.Now().Add(timeout))
	c.Write([]byte{5, code, 0, 1, 0, 0, 0, 0, 0, 0})
}

func bridge(c net.Conn, q *quic.Stream) {
	// ponytail: one-hour hard lifetime, not sliding idle timeout.
	c.SetDeadline(time.Now().Add(time.Hour))
	q.SetDeadline(time.Now().Add(time.Hour))
	var wg sync.WaitGroup
	wg.Add(2)
	abort := func() {
		c.Close()
		q.CancelRead(1)
		q.CancelWrite(1)
	}
	go func() {
		defer wg.Done()
		if _, e := io.Copy(q, c); e != nil {
			abort()
		} else {
			q.Close()
		}
	}()
	go func() {
		defer wg.Done()
		if _, e := io.Copy(c, q); e != nil {
			abort()
			return
		}
		if t, ok := c.(*net.TCPConn); ok {
			t.CloseWrite()
		}
	}()
	wg.Wait()
	q.CancelRead(0)
	c.Close()
}
