package proxy

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"
)

func TestPanelUDPStatsProtected(t *testing.T) {
	p := &panel{sessions: map[string]time.Time{"valid": time.Now().Add(time.Hour), "expired": time.Now().Add(-time.Second)}}
	for _, cookie := range []string{"", "expired", "valid"} {
		r := httptest.NewRequest("GET", "https://panel.example/udp-stats", nil)
		if cookie != "" {
			r.Header.Set("Cookie", "homeproxy_session="+cookie)
		}
		w := httptest.NewRecorder()
		p.serve(w, r)
		if cookie != "valid" {
			if w.Code != 401 {
				t.Fatalf("unauthorized %q: %d", cookie, w.Code)
			}
			continue
		}
		if w.Code != 200 || w.Header().Get("Content-Type") != "application/json" || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("stats response: %d %v", w.Code, w.Header())
		}
		var stats UDPStatsSnapshot
		if err := json.Unmarshal(w.Body.Bytes(), &stats); err != nil {
			t.Fatal(err)
		}
	}
	r := httptest.NewRequest("POST", "https://panel.example/udp-stats", nil)
	r.Header.Set("Cookie", "homeproxy_session=valid")
	w := httptest.NewRecorder()
	p.serve(w, r)
	if w.Code != 405 {
		t.Fatalf("mutation accepted: %d", w.Code)
	}
}
