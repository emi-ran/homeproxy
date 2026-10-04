package proxy

import (
	"io"
	"net"
	"sync"
	"time"

	"github.com/quic-go/quic-go"
)

const timeout = 10 * time.Second
const maxSessions = 128

var qc = &quic.Config{
	EnableDatagrams:       true,
	MaxIdleTimeout:        20 * time.Second,
	KeepAlivePeriod:       5 * time.Second,
	MaxIncomingStreams:    128,
	MaxIncomingUniStreams: -1,
}

type hello struct {
	Token, ID string
	Priority  int
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
