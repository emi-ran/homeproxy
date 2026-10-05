package proxy

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"html/template"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type panel struct {
	s                    *server
	ctx                  context.Context
	mu                   sync.Mutex
	password             [32]byte
	sessions             map[string]time.Time
	nextLogin            time.Time
	host, basePort, path string
}

func (s *server) startPanel(ctx context.Context, addr, socks, state, password string) error {
	if len(password) < 16 {
		return errors.New("HOMEPROXY_PANEL_PASSWORD must contain at least 16 bytes")
	}
	host, port, err := net.SplitHostPort(socks)
	if err != nil {
		return err
	}
	p := &panel{s: s, ctx: ctx, password: sha256.Sum256([]byte(password)), sessions: make(map[string]time.Time), host: host, basePort: port, path: filepath.Join(state, "routes.json")}
	b, err := os.ReadFile(p.path)
	if err == nil {
		var routes map[string]string
		if err = json.Unmarshal(b, &routes); err != nil {
			return err
		}
		for port, id := range routes {
			if err = p.apply(port, id, false); err != nil {
				return err
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	h := &http.Server{Handler: http.HandlerFunc(p.serve), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 8192}
	go func() { <-ctx.Done(); h.Close() }()
	go h.Serve(l)
	return nil
}

func (p *panel) apply(port, id string, persist bool) error {
	n, err := strconv.Atoi(port)
	if err != nil || n < 1024 || n > 65535 || strconv.Itoa(n) != port || len(id) < 1 || len(id) > 64 || strings.ContainsAny(id, "\r\n") {
		return errors.New("invalid port or agent ID")
	}
	p.s.mu.Lock()
	_, exists := p.s.routes[port]
	count := len(p.s.routes)
	p.s.mu.Unlock()
	if !exists && count >= 32 {
		return errors.New("route capacity reached")
	}
	var listener net.Listener
	if !exists && port != p.basePort {
		listener, err = net.Listen("tcp", net.JoinHostPort(p.host, port))
		if err != nil {
			return err
		}
	}
	committed := false
	defer func() {
		if listener != nil && !committed {
			listener.Close()
		}
	}()
	p.s.mu.Lock()
	defer p.s.mu.Unlock()
	old, existed := p.s.routes[port]
	p.s.routes[port] = id
	if persist {
		b, _ := json.MarshalIndent(p.s.routes, "", "  ")
		if err = os.MkdirAll(filepath.Dir(p.path), 0700); err == nil {
			var f *os.File
			f, err = os.CreateTemp(filepath.Dir(p.path), ".routes-*")
			if err == nil {
				name := f.Name()
				defer os.Remove(name)
				_, err = f.Write(b)
				if err == nil {
					err = f.Sync()
				}
				closeErr := f.Close()
				if err == nil {
					err = closeErr
				}
				if err == nil {
					err = os.Rename(name, p.path)
				}
			}
		}
		if err != nil {
			if existed {
				p.s.routes[port] = old
			} else {
				delete(p.s.routes, port)
			}
			return err
		}
	}
	committed = true
	if listener != nil {
		p.s.serveSOCKSListener(p.ctx, listener)
	}
	return nil
}

func (p *panel) serve(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; form-action 'self'; frame-ancestors 'none'")
	if r.URL.Path != "/" && r.URL.Path != "/login" && r.URL.Path != "/route" && r.URL.Path != "/logout" {
		http.NotFound(w, r)
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	now := time.Now()
	for k, expiry := range p.sessions {
		if now.After(expiry) {
			delete(p.sessions, k)
		}
	}
	cookie, _ := r.Cookie("homeproxy_session")
	session := ""
	if cookie != nil {
		session = cookie.Value
	}
	authorized := p.sessions[session].After(now)
	if r.Method == "POST" {
		r.Body = http.MaxBytesReader(w, r.Body, 4096)
		if err := r.ParseForm(); err != nil {
			http.Error(w, "Invalid request", 400)
			return
		}
		if r.URL.Path == "/login" {
			if now.Before(p.nextLogin) {
				http.Error(w, "Try again later", 429)
				return
			}
			p.nextLogin = now.Add(time.Second)
			h := sha256.Sum256([]byte(r.PostForm.Get("password")))
			if subtle.ConstantTimeCompare(h[:], p.password[:]) != 1 {
				http.Error(w, "Invalid password", 401)
				return
			}
			if len(p.sessions) >= 32 {
				http.Error(w, "Session capacity reached", 429)
				return
			}
			b := make([]byte, 32)
			if _, err := rand.Read(b); err != nil {
				http.Error(w, "Login failed", 500)
				return
			}
			session = hex.EncodeToString(b)
			p.sessions[session] = now.Add(8 * time.Hour)
			// Domain access uses HTTPS; loopback HTTP is allowed for local development only.
			host, _, _ := net.SplitHostPort(r.Host)
			if host == "" {
				host = r.Host
			}
			secure := host != "localhost" && host != "127.0.0.1" && host != "[::1]" && host != "::1"
			http.SetCookie(w, &http.Cookie{Name: "homeproxy_session", Value: session, Path: "/", HttpOnly: true, Secure: secure, SameSite: http.SameSiteStrictMode, MaxAge: 28800})
		} else {
			if !authorized {
				http.Error(w, "Unauthorized", 401)
				return
			}
			if subtle.ConstantTimeCompare([]byte(r.PostForm.Get("csrf")), []byte(session)) != 1 {
				http.Error(w, "Invalid CSRF token", 403)
				return
			}
			if r.URL.Path == "/logout" {
				delete(p.sessions, session)
			} else if r.URL.Path == "/route" {
				if err := p.apply(r.PostForm.Get("port"), r.PostForm.Get("id"), true); err != nil {
					http.Error(w, "Route could not be saved; check port and writable state volume", 400)
					return
				}
			} else {
				http.Error(w, "Method not allowed", 405)
				return
			}
		}
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if r.Method != "GET" || r.URL.Path != "/" {
		http.Error(w, "Method not allowed", 405)
		return
	}
	if !authorized {
		io.WriteString(w, loginPage)
		return
	}
	p.s.mu.Lock()
	routes := make(map[string]string)
	for k, v := range p.s.routes {
		routes[k] = v
	}
	agents := make([]string, 0)
	for id := range p.s.agents {
		agents = append(agents, id)
	}
	p.s.mu.Unlock()
	panelTemplate.Execute(w, struct {
		CSRF   string
		Routes map[string]string
		Agents []string
	}{session, routes, agents})
}

const loginPage = `<!doctype html><html lang="tr"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>HomeProxy</title><h1>HomeProxy</h1><form method="post" action="/login"><label>Panel şifresi <input name="password" type="password" autocomplete="current-password" required></label><button>Giriş</button></form></html>`

var panelTemplate = template.Must(template.New("panel").Parse(`<!doctype html><html lang="tr"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>HomeProxy</title><style>body{background:#111614;color:#eee;font:16px system-ui;max-width:720px;margin:40px auto;padding:20px}input,button{font:inherit;padding:10px;margin:6px}table{width:100%;text-align:left}td,th{padding:12px;border-bottom:1px solid #444}</style><h1>HomeProxy</h1><h2>Çevrimiçi agent'lar</h2>{{range .Agents}}<p>{{.}}</p>{{else}}<p>Bağlı cihaz yok.</p>{{end}}<h2>Sabit çıkış portları</h2><table><tr><th>SOCKS portu</th><th>Agent ID</th></tr>{{range $port,$id:=.Routes}}<tr><td>{{$port}}</td><td>{{$id}}</td></tr>{{end}}</table><p>Agent çevrimdışıysa bağlantı reddedilir. Başka cihaza failover yapılmaz. Değişiklik yeni oturumlarda geçerlidir.</p><form method="post" action="/route"><input type="hidden" name="csrf" value="{{.CSRF}}"><label>Port <input name="port" type="number" min="1024" max="65535" required></label><label>Agent ID <input name="id" maxlength="64" required></label><button>Kaydet / güncelle</button></form><form method="post" action="/logout"><input type="hidden" name="csrf" value="{{.CSRF}}"><button>Çıkış</button></form></html>`))
