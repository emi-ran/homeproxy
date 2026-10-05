package proxy

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"io"
	"net"
	"time"

	"github.com/quic-go/quic-go"
)

func runAgent(ctx context.Context, addr string, t *tls.Config, token, id string, priority int, allow bool) error {
	return runAgentStatus(ctx, addr, t, token, id, priority, allow, nil)
}

func runAgentStatus(ctx context.Context, addr string, t *tls.Config, token, id string, priority int, allow bool, connected func()) error {
	c, e := quic.DialAddr(ctx, addr, t, qc)
	if e != nil {
		return e
	}
	defer c.CloseWithError(0, "agent stopped")
	go func() {
		select {
		case <-ctx.Done():
			c.CloseWithError(0, "shutdown")
		case <-c.Context().Done():
		}
	}()
	q, e := c.OpenStreamSync(ctx)
	if e != nil {
		return e
	}
	q.SetDeadline(time.Now().Add(timeout))
	if e = json.NewEncoder(q).Encode(hello{token, id, priority}); e != nil {
		return e
	}
	b := make([]byte, 1)
	if _, e = io.ReadFull(q, b); e != nil {
		return e
	}
	q.Close()
	q.CancelRead(0)
	if connected != nil {
		connected()
	}
	go agentDatagrams(c)
	for {
		q, e := c.AcceptStream(ctx)
		if e != nil {
			return e
		}
		go agentSession(c, q, allow)
	}
}

func agentSession(c *quic.Conn, q *quic.Stream, allow bool) {
	q.SetDeadline(time.Now().Add(timeout))
	b := make([]byte, 1)
	if _, e := io.ReadFull(q, b); e != nil {
		q.CancelRead(1)
		q.Close()
		return
	}
	if b[0] == 3 {
		agentUDP(c, q, allow)
		return
	}
	if b[0] != 1 {
		q.Close()
		q.CancelRead(1)
		return
	}
	addr, e := readAddress(q)
	if e == nil {
		ctx, cancel := context.WithTimeout(c.Context(), timeout)
		defer cancel()
		addr, e = safeTarget(ctx, addr, allow)
	}
	var dst net.Conn
	if e == nil {
		dst, e = net.DialTimeout("tcp", addr, timeout)
	}
	if e != nil {
		q.Write([]byte{1})
		q.Close()
		q.CancelRead(1)
		return
	}
	a, e := encodeAddress(dst.LocalAddr().String())
	if e != nil {
		dst.Close()
		q.Close()
		q.CancelRead(1)
		return
	}
	if _, e = q.Write(append([]byte{0}, a...)); e != nil {
		dst.Close()
		q.Close()
		q.CancelRead(1)
		return
	}
	q.SetDeadline(time.Time{})
	done := make(chan struct{})
	go func() {
		select {
		case <-c.Context().Done():
			dst.Close()
		case <-done:
		}
	}()
	bridge(dst, q)
	close(done)
}
