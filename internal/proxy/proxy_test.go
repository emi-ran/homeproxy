package proxy

import "testing"

func TestAutomaticSticky(t *testing.T) {
	s := newServer("secret", "automatic")
	s.agents["b"] = &agentPeer{id: "b", priority: 2}
	if s.choose().id != "b" {
		t.Fatal("initial")
	}
	s.agents["a"] = &agentPeer{id: "a", priority: 1}
	if s.choose().id != "b" {
		t.Fatal("not sticky")
	}
	delete(s.agents, "b")
	if s.choose().id != "a" {
		t.Fatal("no failover")
	}
}
func TestRouting(t *testing.T) {
	s := newServer("secret", "priority")
	s.agents["a"] = &agentPeer{id: "a", priority: 2}
	s.agents["b"] = &agentPeer{id: "b", priority: 1}
	if p := s.choose(); p == nil || p.id != "b" {
		t.Fatal("priority must choose b")
	}
	if err := s.selectAgent("a"); err != nil {
		t.Fatal(err)
	}
	if p := s.choose(); p == nil || p.id != "a" {
		t.Fatal("manual must choose a")
	}
	delete(s.agents, "a")
	if p := s.choose(); p == nil || p.id != "b" {
		t.Fatal("fallback must choose b")
	}
	if err := s.selectAgent("missing"); err == nil {
		t.Fatal("unknown manual target accepted")
	}
}
