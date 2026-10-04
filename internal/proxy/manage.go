package proxy

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"time"
)

type manageRequest struct{ Token, ID string }

func (s *server) listenManagement(ctx context.Context, path string) error {
	if !filepath.IsAbs(path) {
		return errors.New("absolute management socket path required")
	}
	l, e := net.Listen("unix", path)
	if e != nil {
		return e
	}
	if e = os.Chmod(path, 0600); e != nil {
		l.Close()
		return e
	}
	go func() { <-ctx.Done(); l.Close() }()
	go func() { // Sequential, bounded requests: local control only.
		for {
			c, e := l.Accept()
			if e != nil {
				return
			}
			c.SetDeadline(time.Now().Add(2 * time.Second))
			var r manageRequest
			e = json.NewDecoder(io.LimitReader(c, 1024)).Decode(&r)
			if e == nil && subtle.ConstantTimeCompare([]byte(r.Token), []byte(s.token)) == 1 {
				e = s.selectAgent(r.ID)
			} else {
				e = errors.New("unauthorized")
			}
			if e == nil {
				io.WriteString(c, "ok\n")
			} else {
				io.WriteString(c, "rejected\n")
			}
			c.Close()
		}
	}()
	return nil
}
