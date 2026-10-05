package proxy

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPanelAuthRoutes(t *testing.T) {
	if strings.Contains(strings.ToLower(loginPage), "homeproxy") || !strings.Contains(loginPage, "<title>Giriş</title>") {
		t.Fatal("login page exposes product name")
	}
	s := newServer("test-token", "priority")
	p := &panel{s: s, ctx: context.Background(), password: sha256.Sum256([]byte("panel-password-123")), sessions: make(map[string]time.Time), host: "127.0.0.1", basePort: "1080", path: filepath.Join(t.TempDir(), "routes.json")}
	post := func(path string, values url.Values, cookie string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("POST", "https://panel.example"+path, strings.NewReader(values.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if cookie != "" {
			r.Header.Set("Cookie", cookie)
		}
		w := httptest.NewRecorder()
		p.serve(w, r)
		return w
	}
	if w := post("/route", url.Values{"port": {"1080"}, "id": {"telefon"}}, ""); w.Code != 401 {
		t.Fatal(w.Code)
	}
	if w := post("/login", url.Values{"password": {"wrong"}}, ""); w.Code != 401 {
		t.Fatal(w.Code)
	}
	p.nextLogin = time.Time{}
	w := post("/login", url.Values{"password": {"panel-password-123"}}, "")
	if w.Code != 303 {
		t.Fatal(w.Code)
	}
	c := w.Result().Cookies()[0]
	if !c.Secure || !c.HttpOnly {
		t.Fatal("unsafe session cookie")
	}
	cookie := c.Name + "=" + c.Value
	if w = post("/route", url.Values{"port": {"1080"}, "id": {"telefon"}}, cookie); w.Code != 403 {
		t.Fatal(w.Code)
	}
	if w = post("/route", url.Values{"port": {"1080"}, "id": {"telefon"}, "csrf": {c.Value}}, cookie); w.Code != 303 {
		t.Fatal(w.Code)
	}
	if s.chooseForPort("1080") != nil {
		t.Fatal("offline route fell back")
	}
	s.agents["ev-pc"] = &agentPeer{id: "ev-pc"}
	if s.chooseForPort("1080") != nil {
		t.Fatal("route escaped to PC")
	}
	s.agents["telefon"] = &agentPeer{id: "telefon"}
	if s.chooseForPort("1080").id != "telefon" {
		t.Fatal("wrong agent")
	}
	b, err := os.ReadFile(p.path)
	if err != nil {
		t.Fatal(err)
	}
	var routes map[string]string
	if err = json.Unmarshal(b, &routes); err != nil || routes["1080"] != "telefon" {
		t.Fatal("route not persisted")
	}
	if w = post("/logout", url.Values{"csrf": {c.Value}}, cookie); w.Code != 303 {
		t.Fatal(w.Code)
	}
	if w = post("/route", url.Values{"csrf": {c.Value}}, cookie); w.Code != 401 {
		t.Fatal("logout did not revoke session")
	}
}

func TestPanelAgentIPs(t *testing.T) {
	serverConn, _ := quicPair(t)
	agents := panelAgents(map[string]*agentPeer{
		"telefon": {conn: serverConn},
		"ev-pc":   {conn: serverConn},
		"unknown": {},
	})
	if agents[0].ID != "ev-pc" || agents[0].IP != "127.0.0.1" || !agents[0].SharedIP || !agents[1].SharedIP || agents[2].SharedIP {
		t.Fatalf("unexpected agents: %+v", agents)
	}
	var html bytes.Buffer
	if err := panelTemplate.Execute(&html, struct {
		CSRF   string
		Routes map[string]string
		Agents []panelAgent
	}{"test", map[string]string{"1080": "ev-pc"}, agents}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(html.String(), "Bağlantı IP: 127.0.0.1") || !strings.Contains(html.String(), "Aynı IP başka cihazda") {
		t.Fatal("IP missing from panel")
	}
}
