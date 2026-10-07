# HomeProxy

**SOCKS5 proxy traffic, carried over an authenticated QUIC tunnel and routed through your own devices.**

[English](README.md) · [Türkçe](README.tr.md)

HomeProxy separates the proxy entry point from the internet exit point. Run the server on a VPS or container host, then connect a Windows or Android agent from the network you want to use. Clients connect to the server's SOCKS5 port; destination connections and DNS resolution happen on the agent, not on the VPS.

> **Keep SOCKS5 private.** The SOCKS5 listener has no client authentication. The shared token authenticates agents, not SOCKS5 clients. Do not publish proxy ports to the internet.

## Contents

- [How it works](#how-it-works)
- [Build and downloads](#build-and-downloads)
- [Quick start](#quick-start)
- [Docker and Dokploy](#docker-and-dokploy)
- [Management panel and routing](#management-panel-and-routing)
- [Windows and Android agents](#windows-and-android-agents)
- [Configuration reference](#configuration-reference)
- [Security and limitations](#security-and-limitations)
- [Development and verification](#development-and-verification)

## How it works

```text
SOCKS5 client           HomeProxy server             HomeProxy agent         Destination
(private network)  --> (VPS / container host) <====> (Windows / Android) --> (internet)
                       TCP proxy listener           outbound QUIC tunnel
                       UDP relay                    destination DNS + egress
```

- **TCP:** SOCKS5 CONNECT requests are forwarded through QUIC streams.
- **UDP:** SOCKS5 UDP ASSOCIATE traffic is forwarded through QUIC datagrams, including use cases such as ENet-based games. Clients must support SOCKS5 UDP; compatibility depends on client behavior and packet sizes.
- **Outbound agents:** agents initiate the tunnel, so their router normally needs no inbound port forwarding.
- **Multiple devices:** route a SOCKS5 port to a specific agent, or use the default agent selection behavior.
- **TLS identity:** the server can generate and persist its own certificate. Windows GUI and Android agents support SHA-256 certificate pinning; the CLI supports CA-based verification.
- **Destination protection:** non-public destination addresses are blocked by default on the agent.

This is an application proxy, not a system-wide VPN. Only traffic sent through SOCKS5 uses the tunnel.

## Build and downloads

The CLI/server module requires **Go 1.25 or newer**. The separate mobile module requires **Go 1.26**. Flutter and platform SDKs are required only for GUI/APK builds.

```sh
go mod download
go build -trimpath -o bin/homeproxy ./cmd/homeproxy
```

On Windows, build the CLI/GUI launcher with PowerShell:

```powershell
go build -trimpath -o bin/homeproxy.exe ./cmd/homeproxy
```

Cross-compile from a POSIX shell:

```sh
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -o bin/homeproxy-linux-amd64 ./cmd/homeproxy
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -o bin/homeproxy-windows-amd64.exe ./cmd/homeproxy
```

The manual [GitHub Actions workflow](.github/workflows/build.yml) offers Windows GUI, Android arm64 APK, and CLI builds. Run **HomeProxy Build Workflows** from the repository's Actions tab and download the artifacts from that run. Extract the entire Windows GUI artifact; its DLLs and `data/` directory are required.

See [app/README.md](app/README.md) for local GUI/APK builds and packaging.

## Quick start

The following example uses a Linux server and Windows CLI agent. Replace `proxy.example.com` with your server's reachable address. Allow inbound **UDP 4433** on the server; keep TCP 1080 on loopback or a trusted private network.

### 1. Start the server

Use the same strong random token on both machines, with at least **16 bytes**. Load it through your shell or secret manager; do not commit it.

```sh
export HOMEPROXY_TOKEN='REPLACE_WITH_A_STRONG_RANDOM_TOKEN'
./bin/homeproxy server -quic 0.0.0.0:4433 -socks 127.0.0.1:1080 -state-dir data -admin-socket "$PWD/data/admin.sock"
```

On first start, the server creates `data/server-tls.pem`, containing its certificate and private key. Preserve this directory across restarts and upgrades. The server logs its leaf certificate's SHA-256 fingerprint. The example puts the management socket in that writable directory; the default `/run/homeproxy/` directory must otherwise already exist and be writable.

### 2. Connect an agent with certificate verification

For the generated self-signed certificate, securely transfer **only its public certificate** to the agent as `server-cert.pem`. Never copy the combined `server-tls.pem` file to an agent: it also contains the private key. On the server, you can extract the public certificate with OpenSSL:

```sh
openssl x509 -in data/server-tls.pem -out server-cert.pem
```

Then, in Windows PowerShell:

```powershell
$env:HOMEPROXY_TOKEN = 'REPLACE_WITH_A_STRONG_RANDOM_TOKEN'
.\bin\homeproxy.exe agent -quic proxy.example.com:4433 -id home-pc -ca .\server-cert.pem -server-name homeproxy
```

`homeproxy` is a DNS name in the generated certificate; it need not match the tunnel address when explicitly supplied with `-server-name`. For your own CA-issued certificate, use its actual DNS name and appropriate trust roots instead.

Windows GUI and Android users can enter the fingerprint from trusted server logs instead of installing a CA certificate. CLI `agent` has no fingerprint flag.

### 3. Test through SOCKS5

Run this **on the server**, or adapt the address for a client on the same trusted private network:

```sh
curl --socks5-hostname 127.0.0.1:1080 https://ifconfig.me
```

The returned address should be the agent network's public exit IP. `--socks5-hostname` sends the destination hostname through the proxy rather than resolving it on the client. This checks TCP egress, not UDP or game compatibility.

## Docker and Dokploy

### Docker Compose

The supplied [compose.yaml](compose.yaml) mounts a token file at `/secrets/token`, persists TLS state, and publishes only UDP 4433. It **does not enable the management panel**.

1. Create `secrets/token` containing your shared token. Ensure UID 10001 in the container can read it, while restricting access to other users.
2. Optionally set `HOMEPROXY_MAX_AGENTS=5` in the root `.env` file; Compose defaults to 2 when omitted or empty.
3. Start the service:

```sh
docker compose up -d --build
docker compose logs -f homeproxy
```

Connect client containers to the appropriate shared private network and use `homeproxy:1080`. Do not publish TCP 1080. UDP-capable clients must also be able to reach the dynamically assigned UDP relay ports; access to TCP 1080 alone is insufficient.

### Dokploy Application

Use a **Dockerfile-based Application**. Dokploy Application settings are separate from the local Compose configuration.

| Setting | Value / requirement |
| --- | --- |
| Source | This repository and your chosen branch |
| Build type | Root `Dockerfile` |
| Environment | `HOMEPROXY_TOKEN`, `HOMEPROXY_PANEL_PASSWORD`; optionally `HOMEPROXY_MAX_AGENTS` |
| Public transport | Publish `4433:4433/udp` in host mode |
| SOCKS5 | Keep 1080 and any additional routed ports on the private container network |
| Panel domain | HTTPS reverse proxy to container port 3000; do not expose it as public plain HTTP |
| Persistent mount | Named volume `homeproxy-state` at `/var/lib/homeproxy` |

Both the token and the **separate panel password** must contain at least 16 bytes. The Dockerfile enables the panel by default; a missing/short panel password prevents successful startup.

Preserve the volume on redeploy. Bind mounts must be writable by UID 10001. Do not share one state volume between independent servers. Clients should use the actual service name shown in your deployment, for example `homeproxy-service:1080`, without SOCKS username/password.

## Management panel and routing

The panel shows connected agents and lets you add or update fixed SOCKS5 port-to-agent mappings. For a local server, enable it explicitly:

```sh
export HOMEPROXY_PANEL_PASSWORD='REPLACE_WITH_A_SEPARATE_RANDOM_PASSWORD'
./bin/homeproxy server -quic 0.0.0.0:4433 -socks 127.0.0.1:1080 -state-dir data -admin-socket "$PWD/data/admin.sock" -panel 127.0.0.1:3000
```

This also requires the shared token configured above. For remote access, use an HTTPS reverse proxy; do not publish the plain HTTP listener directly.

Example mappings:

| SOCKS5 port | Agent ID |
| --- | --- |
| 1080 | `home-pc` |
| 1081 | `phone` |

- IDs must exactly match the agent configuration.
- A mapped port uses only that agent for TCP and UDP. If it is offline, requests fail rather than switching devices.
- Changes affect new sessions, not existing connections.
- Up to 32 mappings are stored in `routes.json` in the state directory. The panel currently supports adding/updating, not deletion.
- An unmapped default port follows server selection behavior. Map 1080 explicitly if its exit device must stay fixed.
- Displayed agent IPs are the QUIC peer addresses seen by the server, not independently verified destination-side exit IPs.

Panel sessions expire after eight hours or a server restart. Management operations require CSRF tokens; HTTPS sessions use Secure, HttpOnly, SameSite=Strict cookies. Login attempts are globally rate-limited. These controls do not make a public SOCKS5 listener safe.

## Windows and Android agents

### Windows GUI and service

Run `homeproxy.exe` without arguments from the complete Windows GUI package. It launches the sibling `homeproxy-gui.exe`; the CLI binary alone does not include the GUI.

The GUI controls the `HomeProxyAgent` Windows service rather than starting a separate tunnel. Service installation, removal, start, and stop require consent and UAC. The service uses a low-privilege virtual account, automatic startup, and encrypted settings protected by DPAPI and file ACLs.

- Enter the server address, unique agent ID, shared token, and trusted SHA-256 fingerprint.
- Closing the GUI does not stop the service or tunnel.
- Disconnecting the tunnel persists disabled intent; stopping the Windows service is a different operation.
- When migrating from a console agent or old Startup script, remove/stop the old setup yourself. The installer does not do so automatically.

See [Windows agent documentation](docs/WINDOWS_AGENT.md) for service commands, permissions, packaging, and validation limits. Full SCM/UAC and boot/crash-recovery validation requires a disposable Windows environment.

### Android

The Flutter app uses a Kotlin foreground service and the Go tunnel core. It requires **Android 8.0 (API 26) or newer on arm64**, neither root nor a VPN profile, and proxies only server-requested traffic. The current Gradle configuration signs even release APKs with the development/debug key; Actions APK artifacts are not store-ready distribution packages.

1. Enter the server address, unique agent ID, shared token, and trusted certificate fingerprint.
2. Start the agent from the app. Settings are encrypted with Android Keystore and restored on later launches; restoring settings does not automatically connect.
3. Optionally add the HomeProxy tile to Quick Settings to start/stop using saved settings.

Stop the tunnel before changing connection settings. The app, notification, and Quick Settings tile control the same service. There is no automatic connection after boot/process death, and Android/OEM battery policies can interrupt background operation.

Build instructions: [app/README.md](app/README.md). Tile behavior and outstanding device validation: [Quick Settings guide](docs/ANDROID_QUICK_SETTINGS.md).

## Configuration reference

The CLI does **not** automatically load `.env`. Compose interpolation and Dokploy Environment settings are separate mechanisms.

### Environment variables

| Variable | Purpose / default |
| --- | --- |
| `HOMEPROXY_TOKEN` | Shared agent authentication token; at least 16 bytes |
| `HOMEPROXY_MAX_AGENTS` | Concurrent agent limit; default 2 when unset; must be a positive integer |
| `HOMEPROXY_PANEL_PASSWORD` | Separate panel password; at least 16 bytes when panel is enabled |
| `HOMEPROXY_CERT` / `HOMEPROXY_KEY` | Optional PEM **contents** for server certificate/key; supply both |

Token precedence is `-token`, then `-token-file`, then `HOMEPROXY_TOKEN`. Prefer environment/secret files over command-line secrets, which may be visible in process listings or shell history. Explicit `-cert`/`-key` paths take precedence over certificate environment variables. Invalid certificate input fails startup rather than silently generating a replacement.

The agent limit is read at startup; changing it requires restart. Reconnecting with an existing ID replaces that ID's old connection, even at capacity. Use distinct IDs for distinct devices.

### Common CLI options

| Option | Applies to | Default / purpose |
| --- | --- | --- |
| `-quic` | server, agent | `127.0.0.1:4433`; listen address or remote tunnel address |
| `-token-file` | server, agent, select | Read shared token from a file |
| `-token` | server, agent, select | Explicit shared token; avoid for routine secret handling |
| `-socks` | server | `127.0.0.1:1080`; SOCKS5 bind address |
| `-state-dir` | server | `data`; persistent certificate and panel routing state |
| `-panel` | server | Disabled; private HTTP panel bind address |
| `-cert` / `-key` | server | PEM certificate/key file paths; supply both |
| `-mode` | server | `priority`; accepts `priority`, `automatic`, `manual` |
| `-id` | agent, select | Required agent ID / selection target |
| `-priority` | agent | `100`; lower numbers take precedence |
| `-ca` | agent | CA PEM file; otherwise system trust roots |
| `-server-name` | agent | Required for verified CLI TLS; certificate DNS name |
| `-insecure` | agent | `false`; skips server identity verification—unsafe on untrusted networks |
| `-allow-private` | agent | `false`; dangerous private-destination override for test fixtures |
| `-admin-socket` | server, select | `/run/homeproxy/admin.sock`; local Unix management socket |

For unmapped ports, `priority` chooses the lowest priority value (ID breaks ties), while `automatic` retains its selected agent while available. A selection made with `select` takes precedence. The current `manual` mode also falls back to priority when the selected agent is unavailable; use fixed panel mappings when failover is not acceptable.

On a host supporting Unix sockets, select a connected agent locally:

```sh
./bin/homeproxy select -id home-pc -admin-socket "$PWD/data/admin.sock"
```

This requires the shared token as well as access to the socket. For CLI option help, use `homeproxy server -h` or `homeproxy agent -h`.

## Security and limitations

- **Trust the private client network.** SOCKS5 traffic between client and server is not itself encrypted or authenticated by HomeProxy. QUIC protects the server-agent tunnel.
- **Verify server identity.** Obtain certificates/fingerprints through an independent trusted channel. `-insecure` preserves encryption but lets an impersonating server capture the token and traffic; it is not a safe fix for certificate errors.
- **Protect persistent state.** `server-tls.pem` contains a private key. Losing it changes the server fingerprint; update agent trust explicitly. Corrupt state is not silently regenerated.
- **Keep destination protections enabled.** Private, loopback, link-local, and shared address ranges are blocked by default. This is a defense, not a blanket guarantee against all abuse by trusted clients.
- **UDP is bounded.** The current tunnel datagram limit is 1452 bytes including the 8-byte tunnel header. Oversized packets are dropped, not fragmented by the tunnel. Path MTU and SOCKS5 overhead affect usable payload size.
- **No per-account quotas.** Agent count limits and fixed routing do not enforce third-party game/account limits or ensure unique public IPs. Devices on the same network may share an exit IP.
- **No uptime guarantee.** NAT changes, firewalls, mobile power management, and transport conditions can interrupt tunnels. Physical device and deployment-specific testing remains necessary.

## Development and verification

From the repository root:

```sh
go test ./...
go vet ./...
go test -race ./...
python scripts/check_android_tile.py
python scripts/check_android_restart.py
```

The race detector requires a supported platform and C toolchain. Python checks inspect Android source contracts; they do not replace Kotlin/APK builds or device tests. The mobile module and Flutter app have separate checks documented in [app/README.md](app/README.md).

| Path | Contents |
| --- | --- |
| `cmd/homeproxy/` | CLI entry point and Windows launcher |
| `internal/proxy/` | QUIC tunnel, SOCKS5, routing, panel, destination checks |
| `internal/windowsagent/` | Windows service, encrypted settings, local IPC |
| `mobile/` | Go mobile bindings; separate Go module |
| `app/` | Flutter UI and native Android/Windows integration |
| `scripts/` | Source-contract verification scripts |
| `docs/` | Platform guides, design notes, and historical verification records |

Historical validation and platform-specific caveats are recorded in [VERIFY.md](docs/VERIFY.md), [PANEL_VERIFY.md](docs/PANEL_VERIFY.md), [ANDROID_VERIFY.md](docs/ANDROID_VERIFY.md), and [WINDOWS_AGENT.md](docs/WINDOWS_AGENT.md). They are records of specific checks, not proof that every deployment or current build has been tested.
