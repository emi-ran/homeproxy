package proxy

import "testing"

func TestAdmissionSharedAcrossRoutesAndReleased(t *testing.T) {
	s := newServer("test", "priority")
	var releases []func()
	for i := 0; i < maxSessions; i++ {
		release, ok := s.admit(nil)
		if !ok {
			t.Fatalf("legitimate slot %d rejected", i)
		}
		releases = append(releases, release)
	}
	if _, ok := s.admit(nil); ok {
		t.Fatal("global capacity exceeded")
	}
	releases[0]()
	releases[0]()
	release, ok := s.admit(nil)
	if !ok {
		t.Fatal("released slot leaked")
	}
	defer release()
	if _, ok := s.admit(nil); ok {
		t.Fatal("duplicate release expanded capacity")
	}
	for _, r := range releases[1:] {
		r()
	}
}

func TestAdmissionAgentBudgetPreservesFifteenBots(t *testing.T) {
	s := newServer("test", "priority")
	p := &agentPeer{}
	var releases []func()
	// Fifteen bots may each hold multiple TCP and UDP sessions; no packet throttle.
	for i := 0; i < maxSessions; i++ {
		r, ok := s.admit(p)
		if !ok {
			t.Fatalf("agent slot %d rejected", i)
		}
		releases = append(releases, r)
	}
	if _, ok := s.admit(p); ok {
		t.Fatal("agent capacity exceeded")
	}
	for _, r := range releases {
		r()
	}
	if r, ok := s.admit(p); !ok {
		t.Fatal("agent leaked")
	} else {
		r()
	}
}
