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
- `go build -trimpath -o bin/homeproxy .`: success.
- `GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -o bin/homeproxy-windows-amd64.exe .`: success.
- Running Linux binary without role: expected `use server, agent, select` diagnostic; no server launched.
- `git diff --check`: no diagnostics (rechecked after staging).

Host quic-go warning: receive buffer 208 KiB requested 7168 KiB, obtained 416 KiB. Oversize test logged `oversize UDP dropped`, expected. No kernel configuration changes.

No Docker builds/runs, deployment, remote PC egress test, Windows runtime test, Android build or push.
