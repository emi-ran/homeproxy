// Package mobile exposes the narrow Android gomobile API. Network bytes never cross this bridge.
package mobile

import "homeproxy/internal/proxy"

type Agent struct{ core *proxy.MobileAgent }

func NewAgent() *Agent { return &Agent{core: proxy.NewMobileAgent()} }
func (a *Agent) Start(address, id, token, fingerprint string) error {
	return a.core.Start(address, id, token, fingerprint)
}
func (a *Agent) StartWithTLS(address, id, token, fingerprint string, insecure bool) error {
	return a.core.StartWithTLS(address, id, token, fingerprint, insecure)
}
func (a *Agent) Stop()          { a.core.Stop() }
func (a *Agent) Status() string { return a.core.Status() }
