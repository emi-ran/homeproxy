# HomeProxy reduced headless MVP

Go server + outbound agent. RFC1928 SOCKS5 TCP CONNECT uses QUIC bidirectional streams; UDP ASSOCIATE uses QUIC datagrams. No VDS direct target dial: only agents resolve and dial destinations. Maximum two authenticated agents; shared secret does not provide separate per-agent identity/enrollment. Agent IDs cannot replace live peers. QUIC TLS 1.3 validates certificate chain and DNS name; no insecure mode.

## Build and local use

```
GOMAXPROCS=2 GOFLAGS=-p=1 go test ./...
GOMAXPROCS=2 GOFLAGS=-p=1 go test -race ./...
go vet ./...
go build -o bin/homeproxy .
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o bin/homeproxy-windows-amd64.exe .
```

Use random shared token at least 16 bytes via HOMEPROXY_TOKEN or `-token-file`. Do not pass token in argv. Restrict token/key files to service user. No token logging. Examples assume TLS certificate valid for `proxy.example.com`, private key and token provisioned separately:

```
mkdir -m 700 -p "$HOME/.homeproxy"
bin/homeproxy server -quic 0.0.0.0:4433 -socks 127.0.0.1:1080 -cert cert.pem -key key.pem -token-file token -admin-socket "$HOME/.homeproxy/admin.sock"
bin/homeproxy agent -quic proxy.example.com:4433 -server-name proxy.example.com -id pc-a -priority 10 -token-file token
bin/homeproxy agent -quic proxy.example.com:4433 -server-name proxy.example.com -id pc-b -priority 20 -token-file token
bin/homeproxy select -id pc-b -token-file token -admin-socket "$HOME/.homeproxy/admin.sock"
```

Agent uses system roots or explicit `-ca roots.pem`. No public management port. Server management Unix socket is mode 0600, requires shared token, accepts only bounded selection requests. Parent directory must be owner-only. Existing socket path is never deleted by startup. Windows binary runs agent; Unix management server is Linux-oriented. Agent reconnect delay 3 seconds. Stop with Ctrl-C.

## Routing

`-mode priority`: new sessions choose lowest numeric priority, tie broken lexicographic ID. Manual selection overrides until selected peer disappears; recovered same ID is selected again.

`-mode automatic`: first choice lowest priority, then sticky to healthy choice even if higher-priority peer returns; death chooses and sticks to fallback. Manual selection changes sticky choice.

`-mode manual`: startup chooses priority fallback until local selection; selection overrides while healthy. Missing selection falls back by priority. Thus manual never intentionally disables healthy fallback.

TCP and UDP associations pin exact QUIC connection, not ID. Agent loss closes old controls/sockets/streams; sessions are never replayed or migrated. New sessions use healthy fallback; no agent returns SOCKS failure, never direct egress. Network loss is detected through QUIC idle timeout (20 seconds), not instantly.

## Bounds and UDP behavior

QUIC datagram application maximum **1100 bytes including 8-byte association ID**. SOCKS UDP header included, so payload maximum is 1082 bytes for IPv4, 1070 for IPv6, or `1085 - domain_length` for domains. Path/QUIC limits can reject smaller packets. No application fragmentation; oversize/send failures counted by atomic counter and logged without payload. Loss/reordering inherent to datagrams; no delivery guarantees.

FRAG != 0, nonzero reserved bytes, invalid address types, invalid/truncated headers dropped. UDP sender IP must match TCP peer and sender port must match ASSOCIATE request; unspecified port pins first matching sender. BND address uses TCP listener's actual local interface, so reachable within same internal network without public NAT. TCP control close terminates UDP socket. Agent replies accepted only from one of up to 32 contacted endpoints per association. Session IDs scoped to QUIC connection; queues 32 messages, overflow dropped. 128 concurrent SOCKS sessions, 128 QUIC streams, two agents, four accepted authentication/connection slots. Handshake/connect timeouts 10s; management timeout 2s; UDP idle timeout 60s; session hard lifetime one hour. TCP stream copies use bounded stdlib buffers. No bandwidth quotas/per-client fairness.

Agent destinations default reject loopback/private/link-local/unspecified/multicast IPs, including every DNS answer; dial uses checked numeric IP to avoid re-resolution. `-allow-private` disables that policy only for explicit trusted fixtures. Never enable for untrusted SOCKS clients. SOCKS uses no-auth and MUST remain private; local access permits proxy use. Domain names resolved by selected agent, not server. Domain UDP replies carry numeric source address, per SOCKS semantics.

## Container topology (files only; NOT deployed)

Dockerfile and compose.yaml publish ONLY UDP 4433. SOCKS and dynamically allocated UDP relay ports are unpublished. Clients must share private network and reach both internal SOCKS address and advertised BND IP; UDP cannot work by forwarding SOCKS TCP alone. `private` is internal; `transport` permits QUIC public ingress. Only trusted proxy/client containers may attach. These files were not Docker-built or run.

Existing Mori Dokploy standalone Swarm application uses `dokploy-network`. This Compose private bridge does NOT automatically join Mori or its Swarm tasks. Separate reviewed Swarm overlay/attachable topology, routing and firewall work required before integration. Nothing here changes Mori, Dokploy, services, Docker state or existing networks. Do not publish SOCKS or attach arbitrary containers to transport network. Host firewall policy still required; Compose port publication alone is not complete network isolation.

## Verification and exclusions

Real localhost fixtures generate ephemeral trusted TLS certificates. Tests exercise unauthorized agent rejection, untrusted TLS rejection, TCP echo and half-close, UDP echo, wrong sender/FRAG/oversize rejection, control close, two simultaneous agents with isolated TCP/UDP sessions, pinned agent death and healthy fallback, priority/manual/automatic routing, destination policy, IPv4/IPv6/domain codecs, authenticated local selection. Localhost tests do NOT prove remote PC public egress IP, internet performance or NAT reachability.

No staging/remote-PC/Dokploy deployment run. Android not built. Windows cross-build only, runtime not tested. No mTLS enrollment, GUI, SQLite, installer, OS service, Keystore, DPAPI, traffic obfuscation, or application fragmentation. quic-go only direct dependency; its transitives recorded in go.mod/go.sum. Host emitted quic-go UDP receive-buffer warning; no sysctl/system changes made.
