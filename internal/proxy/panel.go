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
	"sort"
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
	agents := panelAgents(p.s.agents)
	p.s.mu.Unlock()
	panelTemplate.Execute(w, struct {
		CSRF   string
		Routes map[string]string
		Agents []panelAgent
	}{session, routes, agents})
}

type panelAgent struct {
	ID, IP   string
	SharedIP bool
}

// RemoteAddr reflects the current QUIC path, including network migrations.
// It is not a measurement of the agent's egress IP toward every target.
func panelAgents(peers map[string]*agentPeer) []panelAgent {
	agents := make([]panelAgent, 0, len(peers))
	counts := make(map[string]int)
	for id, peer := range peers {
		ip := "Bilinmiyor"
		if peer.conn != nil {
			if host, _, err := net.SplitHostPort(peer.conn.RemoteAddr().String()); err == nil {
				ip = host
			}
		}
		agents = append(agents, panelAgent{ID: id, IP: ip})
		if ip != "Bilinmiyor" {
			counts[ip]++
		}
	}
	for i := range agents {
		agents[i].SharedIP = counts[agents[i].IP] > 1
	}
	sort.Slice(agents, func(i, j int) bool { return agents[i].ID < agents[j].ID })
	return agents
}

const panelStyle = `<style>
:root{color-scheme:dark;--bg:#111614;--surface:#19201c;--line:#303a33;--muted:#a4b0a7;--accent:#c2df8c}*{box-sizing:border-box}body{margin:0;background:var(--bg);color:#edf2ec;font:16px/1.55 system-ui,sans-serif}main{max-width:960px;margin:64px auto;padding:0 24px}h1,h2,p{margin:0}h1{font-size:30px;letter-spacing:-.8px}h2{font-size:19px;margin-bottom:20px}.eyebrow{color:var(--accent);font-size:12px;letter-spacing:2px;font-weight:650;margin-bottom:8px}.muted,.note{color:var(--muted);font-size:14px}.header{display:flex;justify-content:space-between;align-items:center;gap:20px;margin-bottom:32px}.card{background:var(--surface);border:1px solid var(--line);border-radius:14px;padding:28px;margin-bottom:20px}label{display:flex;flex-direction:column;gap:8px;font-size:14px;font-weight:550}input,button{font:inherit;border-radius:8px;padding:12px 14px}input{width:100%;min-width:0;border:1px solid #455147;background:#111614;color:#edf2ec}input:focus-visible,button:focus-visible{outline:2px solid var(--accent);outline-offset:3px}button{border:1px solid transparent;background:var(--accent);color:#192316;cursor:pointer;font-weight:650;min-height:46px}button:hover{background:#d0eaa2}.secondary{background:transparent;color:#d8e1d9;border-color:#455147}.secondary:hover{background:#263028}.agents{display:flex;flex-wrap:wrap;gap:10px}.agent{border:1px solid var(--line);border-radius:8px;padding:9px 14px;display:flex;align-items:center;gap:9px}.dot{height:7px;width:7px;border-radius:50%;background:var(--accent)}.route-form{display:grid;grid-template-columns:140px 1fr auto;gap:16px;align-items:end;margin-top:24px}.note{margin-top:18px}table{width:100%;border-collapse:collapse;text-align:left}th{font-size:12px;text-transform:uppercase;letter-spacing:1px;color:var(--muted);font-weight:550}td,th{padding:13px 8px;border-bottom:1px solid var(--line)}td:first-child{font-family:ui-monospace,monospace;color:var(--accent)}.login{max-width:420px;margin:15vh auto}.login h1{margin-bottom:8px}.login form{display:grid;gap:20px;margin-top:28px}.login .card{padding:32px}.login .muted{margin-top:4px}@media(max-width:600px){main{margin:28px auto;padding:0 16px}.card{padding:20px}.route-form{grid-template-columns:1fr}.header{align-items:flex-start}.login{margin:12vh auto}h1{font-size:27px}}
</style>`

const loginPage = `<!doctype html><html lang="tr"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Giriş</title>` + panelStyle + `</head><body><main class="login"><section class="card"><p class="eyebrow">ÖZEL ERİŞİM</p><h1>Hoş geldin.</h1><p class="muted">Devam etmek için şifreni gir.</p><form method="post" action="/login"><label>Şifre<input name="password" type="password" autocomplete="current-password" required autofocus></label><button>Giriş yap</button></form></section></main></body></html>`

var panelTemplate = template.Must(template.New("panel").Parse(`<!doctype html><html lang="tr"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>HomeProxy</title>` + panelStyle + `</head><body><main>
<header class="header"><div><p class="eyebrow">BAĞLANTI YÖNETİMİ</p><h1>HomeProxy</h1><p class="muted">Cihazlarını ve internet çıkışlarını yönet.</p></div><form method="post" action="/logout"><input type="hidden" name="csrf" value="{{.CSRF}}"><button class="secondary">Çıkış yap</button></form></header>
<section class="card"><h2>Çevrimiçi cihazlar <span class="muted">· {{len .Agents}}</span></h2>
<div class="agents">{{range .Agents}}<div class="agent"><span class="dot" aria-hidden="true"></span><div><strong>{{.ID}}</strong><div class="muted">Bağlantı IP: {{.IP}}</div>{{if .SharedIP}}<div style="color:#ebc681">Aynı IP başka cihazda da kullanılıyor</div>{{end}}</div></div>{{else}}<p class="muted">Henüz bağlı cihaz yok.</p>{{end}}</div>
<p class="note">IP, sunucunun gördüğü QUIC bağlantı adresidir. Oyun hedefindeki çıkış IP’si farklı olabilir. Güncel bilgi için sayfayı yenile.</p></section>
<section class="card"><h2>Sabit çıkış portları</h2><table><thead><tr><th scope="col">SOCKS portu</th><th scope="col">Agent ID</th></tr></thead><tbody>{{range $port,$id:=.Routes}}<tr><td>{{$port}}</td><td>{{$id}}</td></tr>{{else}}<tr><td colspan="2">Henüz port eşlemesi yok.</td></tr>{{end}}</tbody></table>
<form class="route-form" method="post" action="/route"><input type="hidden" name="csrf" value="{{.CSRF}}"><label>SOCKS portu<input name="port" type="number" min="1024" max="65535" placeholder="1081" required></label><label>Agent ID<input name="id" maxlength="64" placeholder="telefon" list="agents" required></label><datalist id="agents">{{range .Agents}}<option value="{{.ID}}">{{end}}</datalist><button>Kaydet / güncelle</button></form>
<p class="note">Aynı portu girerek eşlemeyi güncelleyebilirsin. Değişiklikler yeni oturumlara uygulanır.</p><p class="note">Cihaz çevrimdışıysa bağlantı reddedilir; başka cihaza otomatik geçiş yapılmaz.</p></section></main></body></html>`))
