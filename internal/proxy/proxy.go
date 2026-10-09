package proxy

import (
	"errors"
	"sync"

	"github.com/quic-go/quic-go"
)

type agentPeer struct {
	id       string
	priority int
	conn     *quic.Conn
	udp      sync.Map
	sessions int // guarded by server.mu
}

type server struct {
	mu                    sync.Mutex
	token, mode, selected string
	agents                map[string]*agentPeer
	routes                map[string]string
	maxAgents             int
	sessions              int
}

func newServer(token, mode string) *server {
	return &server{token: token, mode: mode, agents: make(map[string]*agentPeer), routes: make(map[string]string), maxAgents: 2}
}

func (s *server) chooseForPort(port string) *agentPeer {
	s.mu.Lock()
	defer s.mu.Unlock()
	if id, ok := s.routes[port]; ok {
		return s.agents[id]
	}
	return s.chooseLocked()
}

func (s *server) choose() *agentPeer {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.chooseLocked()
}

func (s *server) chooseLocked() *agentPeer {
	if p := s.agents[s.selected]; p != nil {
		return p
	}
	var best *agentPeer
	for _, p := range s.agents {
		if best == nil || p.priority < best.priority || (p.priority == best.priority && p.id < best.id) {
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
