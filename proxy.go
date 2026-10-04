package main

import (
	"errors"
	"github.com/quic-go/quic-go"
	"sync"
)

type agentPeer struct {
	id       string
	priority int
	conn     *quic.Conn
	udp      sync.Map
}
type server struct {
	mu                    sync.Mutex
	token, mode, selected string
	agents                map[string]*agentPeer
}

func newServer(token, mode string) *server {
	return &server{token: token, mode: mode, agents: make(map[string]*agentPeer)}
}
func (s *server) choose() *agentPeer { s.mu.Lock(); defer s.mu.Unlock(); return s.chooseLocked() }
func (s *server) chooseLocked() *agentPeer {
	if p := s.agents[s.selected]; p != nil {
		return p
	}
	var best *agentPeer
	for _, p := range s.agents {
		if best == nil || p.priority < best.priority || p.priority == best.priority && p.id < best.id {
			best = p
		}
	}
	if s.mode == "automatic" && best != nil {
		s.selected = best.id
	}
	return best
}
func (s *server) selectAgent(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.agents[id] == nil {
		return errors.New("agent unavailable")
	}
	s.selected = id
	return nil
}
