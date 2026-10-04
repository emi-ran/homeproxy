package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"testing"
	"time"

	"github.com/quic-go/quic-go"
)

func expectFailureReply(t *testing.T, c net.Conn, code byte) {
	t.Helper()
	b := make([]byte, 3)
	if _, e := io.ReadFull(c, b); e != nil {
		t.Fatalf("missing failure reply: %v", e)
	}
	if !bytes.Equal(b, []byte{5, code, 0}) {
		t.Fatalf("reply %v, want REP %d", b, code)
	}
	if _, e := readAddress(c); e != nil {
		t.Fatalf("invalid failure BND: %v", e)
	}
	c.SetReadDeadline(time.Now().Add(time.Second))
	if _, e := c.Read(make([]byte, 1)); e != io.EOF {
		t.Fatalf("failure not closed: %v", e)
	}
}

func TestUnsupportedAddressReply(t *testing.T) {
	for _, cmd := range []byte{1, 3} {
		t.Run(fmt.Sprint(cmd), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			s := newServer("secret", "priority")
			sa, e := s.listenSOCKS(ctx, "127.0.0.1:0")
			if e != nil {
				t.Fatal(e)
			}
			c, e := net.Dial("tcp", sa)
			if e != nil {
				t.Fatal(e)
			}
			defer c.Close()
			c.SetDeadline(time.Now().Add(time.Second))
			c.Write([]byte{5, 1, 0})
			b := make([]byte, 2)
			if _, e := io.ReadFull(c, b); e != nil {
				t.Fatal(e)
			}
			c.Write([]byte{5, cmd, 0, 7})
			expectFailureReply(t, c, 8)
		})
	}
}

func TestSetupFailureReplies(t *testing.T) {
	for _, cmd := range []byte{1, 3} {
		for _, mode := range []string{"closed", "eof", "status", "timeout", "source"} {
			if mode == "source" && cmd != 3 {
				continue
			}
			t.Run(fmt.Sprintf("%d/%s", cmd, mode), func(t *testing.T) {
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
				agent, e := quic.DialAddr(ctx, qa, ct, qc)
				if e != nil {
					t.Fatal(e)
				}
				defer agent.CloseWithError(0, "test done")
				q, e := agent.OpenStreamSync(ctx)
				if e != nil {
					t.Fatal(e)
				}
				json.NewEncoder(q).Encode(hello{"secret", "a", 1})
				if _, e := io.ReadFull(q, make([]byte, 1)); e != nil {
					t.Fatal(e)
				}
				q.Close()
				q.CancelRead(0)
				waitAgents(t, s, 1)
				if mode == "closed" {
					p := s.choose()
					p.conn.CloseWithError(0, "test closed")
					// Retain stale selection to exercise OpenStreamSync failure, not no-agent path.
					s.mu.Lock()
					s.agents["stale"] = p
					s.mu.Unlock()
				} else {
					go func() {
						stream, e := agent.AcceptStream(ctx)
						if e != nil {
							return
						}
						defer stream.Close()
						defer stream.CancelRead(0)
						if _, e := io.ReadFull(stream, make([]byte, 1)); e != nil {
							return
						}
						if cmd == 1 {
							if _, e := readAddress(stream); e != nil {
								return
							}
						}
						switch mode {
						case "status":
							stream.Write([]byte{1})
						case "timeout":
							<-ctx.Done()
						}
					}()
				}
				target := "127.0.0.1:1234"
				if mode == "source" {
					target = "192.0.2.1:0"
				}
				c := socks(t, sa, cmd, target)
				defer c.Close()
				c.SetDeadline(time.Now().Add(timeout + 2*time.Second))
				code := byte(1)
				if mode == "source" || (mode == "status" && cmd == 1) {
					code = 2
				}
				expectFailureReply(t, c, code)
			})
		}
	}
}

func TestConnectReplyBoundAddress(t *testing.T) {
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
	go runAgent(ctx, qa, ct, "secret", "a", 1, true)
	waitAgents(t, s, 1)
	for _, network := range []string{"tcp4", "tcp6"} {
		t.Run(network, func(t *testing.T) {
			target := "127.0.0.1:0"
			if network == "tcp6" {
				target = "[::1]:0"
			}
			l, e := net.Listen(network, target)
			if e != nil {
				if network == "tcp6" {
					t.Skip(e)
				}
				t.Fatal(e)
			}
			defer l.Close()
			accepted := make(chan net.Conn, 1)
			go func() {
				c, e := l.Accept()
				if e == nil {
					accepted <- c
				}
			}()
			c := socks(t, sa, 1, l.Addr().String())
			defer c.Close()
			bound := reply(t, c)
			dst := <-accepted
			defer dst.Close()
			if bound != dst.RemoteAddr().String() {
				t.Fatalf("BND = %s, destination sees %s", bound, dst.RemoteAddr())
			}
			c.Write([]byte("x"))
			b := make([]byte, 1)
			if _, e := io.ReadFull(dst, b); e != nil || b[0] != 'x' {
				t.Fatalf("payload %q %v", b, e)
			}
		})
	}
}
