# Local verification

2026-10-04, Linux amd64, isolated Go 1.25.3 at `go`.
All commands serialized with `GOMAXPROCS=2 GOFLAGS=-p=1`.

- RED routing: `go test ./...` failed undefined newServer/agentPeer before implementation; GREEN passed.
- RED TLS/TCP integration: failed missing address/QUIC/SOCKS/agent functions; GREEN passed real TLS/TCP half-close test.
- RED UDP association: `go test -run TestUDPAssociation -v` failed EOF with placeholder; GREEN passed real UDP echo and rejection checks.
- RED management authentication: failed missing listenManagement/manageRequest; GREEN passed.
- Later routing/policy/failover regression tests first ran against existing implementation and passed; these were not separate RED cycles.
- Final `go test -count=3 -v ./...`: 9 tests, 27 executions passed; `ok homeproxy 2.087s`.
- Final `go test -race ./...`: `ok homeproxy 2.048s`, no race reports.
- `go vet ./...`: exit 0, no diagnostics.
- `go build -trimpath -o bin/homeproxy ./cmd/homeproxy`: success.
- `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -o bin/homeproxy-windows-amd64.exe ./cmd/homeproxy`: success.
- Running Linux binary without role: expected `use server, agent, select` diagnostic; no server launched.
- `git diff --check`: no diagnostics (rechecked after staging).

Host quic-go warning: receive buffer 208 KiB requested 7168 KiB, obtained 416 KiB. Oversize test logged `oversize UDP dropped`, expected. No kernel configuration changes.

## Security/cleanup review fixes

- RED confirmed shared-range numeric/mapped IPv4 and synthetic DNS answers accepted; GREEN rejects 100.64.0.0/10 through shared TCP/UDP destination policy, preserves adjacent public ranges and explicit fixture override.
- RED TCP reset and QUIC stream reset left bridge blocked; GREEN returns promptly while existing clean half-close test passes.
- RED individual UDP stream EOF/reset left SOCKS control open; GREEN closes control and removes relay registration without killing QUIC connection.
- RED 200ms idle timeout killed sustained one-way UDP; GREEN successful outbound writes refresh idle and independent 900ms hard lifetime still expires under traffic. Durations injected per invocation, no mutable global timeouts.
- RED malformed FRAG packet pinned unspecified UDP source port; GREEN subsequent valid sender succeeds.
- RED Unix SIGTERM killed helper process; GREEN CLI exits normally. syscall.SIGTERM also cross-compiles on Windows; no build-specific production files needed.
- Final `go test -count=1 -v ./...`: `ok homeproxy 21.965s`.
- Final `go test -race -count=1 ./...`: `ok homeproxy 24.219s`, no race reports.
- Final `go vet ./...`, Linux build, Windows amd64 cross-build, `git diff --check`: exit 0. Refreshed `bin/homeproxy` and `bin/homeproxy-windows-amd64.exe` (ignored artifacts).
- Original four-slot/10s registration bound assessed at that baseline; slots then covered entire connections. Current implementation bounds pending authentication only, independently of the configurable active-agent limit. Pending-slot exhaustion can still temporarily reject reconnects; no production DoS guarantee.

No Docker builds/runs, deployment, remote PC egress test, Windows runtime test, Android build or push.

## Configurable agent capacity (2026-10-05, Windows)

- Baseline: clean `c5b758d`; active-agent limit was hardcoded to two.
- `go test ./internal/proxy -run 'Test(MaxAgentsEnv|AgentLimit)$' -count=10`: passed. Real QUIC registration acknowledges two/default and five/configured agents, rejects the next, and replaces the same ID at full capacity. Reconnect checks use registration acknowledgements and connection-close signals, not sleeps.
- Environment tests cover unset default, positive values through platform `int` maximum, empty/zero/negative/malformed/overflow errors before server startup.
- `go test ./internal/proxy`: passed (`22.923s`).
- `go vet ./internal/proxy` and `go build ./cmd/homeproxy`: passed.
- `docker compose config --format json`: parsed successfully; local ignored `.env` supplies `HOMEPROXY_MAX_AGENTS=5` to container environment. No container started.
- `git diff --check`: passed; Git emitted only LF/CRLF conversion warnings.
- Race check blocked: `go test -race ./internal/proxy -run 'Test(MaxAgentsEnv|AgentLimit)$' -count=10` reported `go: -race requires cgo; enable cgo by setting CGO_ENABLED=1`. Environment has `CGO_ENABLED=0`, no `gcc` on PATH; no toolchain changes made.
- No deployment, production environment edits, commit, push, Android diagnostics or adb changes.
