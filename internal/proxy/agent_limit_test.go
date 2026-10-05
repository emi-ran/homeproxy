package proxy

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/quic-go/quic-go"
)

func TestMaxAgentsEnv(t *testing.T) {
	t.Setenv("HOMEPROXY_MAX_AGENTS", "")
	if err := os.Unsetenv("HOMEPROXY_MAX_AGENTS"); err != nil {
		t.Fatal(err)
	}
	if n, err := maxAgentsFromEnv(); err != nil || n != 2 {
		t.Fatalf("unset: got %d, %v", n, err)
	}
	for _, value := range []string{"1", "5", strconv.Itoa(int(^uint(0) >> 1))} {
		t.Run(value, func(t *testing.T) {
			t.Setenv("HOMEPROXY_MAX_AGENTS", value)
			n, err := maxAgentsFromEnv()
			if err != nil || strconv.Itoa(n) != value {
				t.Fatalf("got %d, %v", n, err)
			}
		})
	}
	for _, value := range []string{"", "0", "-1", "five", "1.5", " 5 ", "999999999999999999999999999999"} {
		t.Run("invalid="+value, func(t *testing.T) {
			t.Setenv("HOMEPROXY_MAX_AGENTS", value)
			err := RunCLI([]string{"server", "-token", "0123456789abcdef"})
			if err == nil || !strings.Contains(err.Error(), "HOMEPROXY_MAX_AGENTS") {
				t.Fatalf("expected env validation before startup, got %v", err)
			}
		})
	}
}

func TestAgentLimit(t *testing.T) {
	for _, limit := range []int{2, 5} {
		t.Run(fmt.Sprint(limit), func(t *testing.T) {
			st, ct := testTLS(t)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			s := newServer("secret", "priority")
			if limit == 5 {
				t.Setenv("HOMEPROXY_MAX_AGENTS", "5")
				var err error
				s.maxAgents, err = maxAgentsFromEnv()
				if err != nil {
					t.Fatal(err)
				}
			}
			addr, err := s.listenQUIC(ctx, "127.0.0.1:0", st)
			if err != nil {
				t.Fatal(err)
			}
			connect := func(id string, accepted bool) *quic.Conn {
				t.Helper()
				c, err := quic.DialAddr(ctx, addr, ct, qc)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { c.CloseWithError(0, "test cleanup") })
				q, err := c.OpenStreamSync(ctx)
				if err != nil {
					t.Fatal(err)
				}
				q.SetDeadline(time.Now().Add(3 * time.Second))
				if err := json.NewEncoder(q).Encode(hello{Token: "secret", ID: id}); err != nil {
					t.Fatal(err)
				}
				ack := []byte{255}
				_, err = io.ReadFull(q, ack)
				if accepted {
					if err != nil || ack[0] != 0 {
						t.Fatalf("%s registration: ack=%v err=%v", id, ack, err)
					}
				} else {
					if err == nil {
						t.Fatal("excess agent accepted")
					}
					select {
					case <-c.Context().Done():
					case <-ctx.Done():
						t.Fatal("excess agent not closed")
					}
				}
				return c
			}
			var first *quic.Conn
			for i := 0; i < limit; i++ {
				c := connect(fmt.Sprint(i), true)
				if i == 0 {
					first = c
				}
			}
			connect("extra", false)
			replacement := connect("0", true)
			select {
			case <-first.Context().Done():
			case <-ctx.Done():
				t.Fatal("reconnect did not close old connection")
			}
			// Another acknowledged registration proves capacity and replacement survived
			// old-connection cleanup, without relying on an arbitrary sleep.
			latest := connect("0", true)
			select {
			case <-replacement.Context().Done():
			case <-ctx.Done():
				t.Fatal("second reconnect did not close replaced connection")
			}
			s.mu.Lock()
			count, current := len(s.agents), s.agents["0"]
			s.mu.Unlock()
			if count != limit || current == nil || latest.Context().Err() != nil {
				t.Fatalf("reconnect lost registration: count=%d current=%v", count, current != nil)
			}
			connect("extra", false)
		})
	}
}
