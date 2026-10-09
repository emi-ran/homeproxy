package proxy

import "time"

type reconnectState struct {
	fails         int
	authenticated time.Time
}

func (r *reconnectState) next(now time.Time) time.Duration {
	if !r.authenticated.IsZero() && now.Sub(r.authenticated) >= 30*time.Second {
		r.fails = 0
	}
	r.authenticated = time.Time{}
	d := reconnectDelay(r.fails)
	r.fails++
	return d
}
