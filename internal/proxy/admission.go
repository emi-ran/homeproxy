package proxy

import "sync"

// Global admission bounds accepted sockets across all routes; agent admission
// separately bounds pinned sessions. Neither limits packet rate or throughput.
func (s *server) admit(p *agentPeer) (func(), bool) {
	s.mu.Lock()
	count := &s.sessions
	if p != nil {
		count = &p.sessions
	}
	if *count >= maxSessions {
		s.mu.Unlock()
		return nil, false
	}
	*count++
	s.mu.Unlock()
	var once sync.Once
	return func() { once.Do(func() { s.mu.Lock(); *count--; s.mu.Unlock() }) }, true
}
