package proxy

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

// RunCLI executes the HomeProxy command line interface.
func RunCLI(args []string) error {
	if len(args) < 1 {
		return errors.New("use server, agent, select")
	}
	role := args[0]
	f := flag.NewFlagSet(role, flag.ContinueOnError)
	addr := f.String("quic", "127.0.0.1:4433", "QUIC server address")
	socks := f.String("socks", "127.0.0.1:1080", "private SOCKS bind")
	cert := f.String("cert", "", "server PEM certificate")
	key := f.String("key", "", "server PEM key")
	ca := f.String("ca", "", "agent CA PEM (otherwise system roots)")
	name := f.String("server-name", "", "TLS certificate DNS name")
	tokenFile := f.String("token-file", "", "shared token file (otherwise HOMEPROXY_TOKEN)")
	id := f.String("id", "", "agent ID or selection target")
	priority := f.Int("priority", 100, "lower wins")
	mode := f.String("mode", "priority", "priority, automatic, manual")
	admin := f.String("admin-socket", "/run/homeproxy/admin.sock", "local Unix management socket")
	allow := f.Bool("allow-private", false, "DANGER: permit agent private destinations; test fixtures only")
	if e := f.Parse(args[1:]); e != nil {
		return e
	}
	token := os.Getenv("HOMEPROXY_TOKEN")
	if *tokenFile != "" {
		b, e := os.ReadFile(*tokenFile)
		if e != nil {
			return e
		}
		token = strings.TrimSpace(string(b))
	}
	if len(token) < 16 {
		return errors.New("shared token must contain at least 16 bytes")
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	switch role {
	case "server":
		if *mode != "priority" && *mode != "automatic" && *mode != "manual" {
			return errors.New("invalid routing mode")
		}
		pair, e := tls.LoadX509KeyPair(*cert, *key)
		if e != nil {
			return e
		}
		s := newServer(token, *mode)
		if e = s.listenManagement(ctx, *admin); e != nil {
			return e
		}
		if _, e = s.listenQUIC(ctx, *addr, &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{pair}, NextProtos: []string{"homeproxy/1"}}); e != nil {
			return e
		}
		if _, e = s.listenSOCKS(ctx, *socks); e != nil {
			return e
		}
		log.Print("server ready")
		<-ctx.Done()
		return nil
	case "agent":
		if *id == "" || *name == "" {
			return errors.New("id and server-name required")
		}
		t := &tls.Config{MinVersion: tls.VersionTLS13, ServerName: *name, NextProtos: []string{"homeproxy/1"}}
		if *ca != "" {
			b, e := os.ReadFile(*ca)
			if e != nil {
				return e
			}
			t.RootCAs = x509.NewCertPool()
			if !t.RootCAs.AppendCertsFromPEM(b) {
				return errors.New("invalid CA")
			}
		}
		for ctx.Err() == nil {
			e := runAgent(ctx, *addr, t, token, *id, *priority, *allow)
			if ctx.Err() != nil {
				break
			}
			if e != nil {
				log.Print("agent disconnected; retry in 3s")
			}
			select {
			case <-ctx.Done():
			case <-time.After(3 * time.Second):
			}
		}
		return nil
	case "select":
		c, e := net.DialTimeout("unix", *admin, 2*time.Second)
		if e != nil {
			return e
		}
		defer c.Close()
		c.SetDeadline(time.Now().Add(2 * time.Second))
		if e = json.NewEncoder(c).Encode(manageRequest{token, *id}); e != nil {
			return e
		}
		b, e := io.ReadAll(io.LimitReader(c, 128))
		if e != nil {
			return e
		}
		if string(b) != "ok\n" {
			return errors.New("selection rejected")
		}
		fmt.Print(string(b))
		return nil
	default:
		return errors.New("unknown role")
	}
}
