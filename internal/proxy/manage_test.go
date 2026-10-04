package proxy

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"path/filepath"
	"testing"
)

func TestLocalManagementAuth(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := newServer("secret", "priority")
	s.agents["a"] = &agentPeer{id: "a", priority: 1}
	s.agents["b"] = &agentPeer{id: "b", priority: 2}
	path := filepath.Join(t.TempDir(), "admin.sock")
	if e := s.listenManagement(ctx, path); e != nil {
		t.Fatal(e)
	}
	for _, tc := range []struct{ token, want string }{{"wrong", "a"}, {"secret", "b"}} {
		c, e := net.Dial("unix", path)
		if e != nil {
			t.Fatal(e)
		}
		json.NewEncoder(c).Encode(manageRequest{Token: tc.token, ID: "b"})
		io.ReadAll(c)
		c.Close()
		if s.choose().id != tc.want {
			t.Fatal("management authorization")
		}
	}
}
