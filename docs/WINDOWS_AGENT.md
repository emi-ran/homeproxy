# Windows agent backend / Flutter contract

Go backend and Flutter Windows runner implemented. No service
installation, current-agent shutdown, deployment or power-policy changes were
performed during development.

## Binaries and packaging

- `homeproxy.exe`: Go CLI + no-argument GUI launcher. Existing `agent`, `server`,
  `select` commands unchanged. Windows no-argument launch starts sibling
  `homeproxy-gui.exe`; missing runner gives actionable error. Linux still requires role.
- `homeproxy-service.exe`: same Go build, permanent SCM executable. Never name
  Flutter runner this or `homeproxy.exe`.
- `homeproxy-gui.exe`: Flutter Windows runner; package DLLs, `data/`, Flutter
  assets and Go `homeproxy.exe` alongside it. GUI must not start a second tunnel.

```powershell
go build -trimpath -o bin/homeproxy.exe ./cmd/homeproxy
go build -trimpath -o bin/homeproxy-service.exe ./cmd/homeproxy
go test ./internal/windowsagent -v
go test ./... -timeout 60s
# After frontend wave, from app/:
flutter build windows --release
# Frontend must set Windows CMake BINARY_NAME to homeproxy-gui.
# Ship entire build/windows/x64/runner/Release directory, not exe alone.
```

Backend uses existing `proxy.MobileAgent` and `runAgentStatus` QUIC/TCP/UDP core.
Windows global ID mutex prevents service/console duplicates across sessions;
mutex access errors fail closed. Existing console instance must be stopped by
user before migration; installer never kills it. No VPN driver or power setting changes.

## Installation and privilege boundary

SCM service: `HomeProxyAgent`, automatic start, virtual account
`NT SERVICE\HomeProxyAgent` (not LocalSystem; no interactive desktop). Permanent
executable: Windows KnownFolder ProgramFiles + `HomeProxy\homeproxy-service.exe`.
State: KnownFolder ProgramData + `HomeProxy\`.

Install/uninstall require elevated administrator token. GUI runs unelevated.
For install, GUI captures its current user SID **before elevation**, then uses
`ShellExecuteExW` verb `runas` on packaged Go `homeproxy.exe`, arguments
`service install --user-sid S-1-5-21-...`. SID must resolve to user account, not
group. Never derive owner from elevated helper: UAC may use another account.
GUI waits for elevated helper exit and checks result; UAC cancellation is not success.
No credentials in command-line arguments. GUI must obtain separate user consent
for install/removal/start/stop; these operations are not pipe operations.

Manual equivalent (SID below is placeholder; capture actual unelevated user SID):

```powershell
# Elevated PowerShell; installs but does not start immediately:
.\bin\homeproxy.exe service install --user-sid S-1-5-21-111-222-333-1001
.\bin\homeproxy.exe service start
.\bin\homeproxy.exe service status
.\bin\homeproxy.exe service stop
# Wait until SCM state is 1 (Stopped), then:
.\bin\homeproxy.exe service uninstall
```

CLI SCM controls currently require administrator rights (manager opens service
with full access). GUI uses elevated helper for start/stop, or clearly reports
not running and offers consented start. Do not elevate whole Flutter process.
Uninstall refuses running service, deletes SCM entry and permanent exe; keeps
encrypted settings/owner SID. Reinstall explicitly replaces authorized user SID.
Existing executable path is not overwritten. Partial installer errors attempt
rollback of newly created service/executable; errors may require administrator
inspection. No auto-update mechanism.

SCM recovery: restart after 5 seconds, then 30 seconds, repeat 30 seconds; reset
failure count after 24 hours. Only process crashes trigger recovery, not reported
non-crash errors or SCM Stop. Service startup errors return service-specific codes
1 (paths), 2 (settings/pipe/agent), 3 (pipe listener). Manual SCM stop is not
restarted until explicit Start or next boot. Use IPC `disconnect` for persistent
disabled tunnel: service stays running, enabled=false survives boot/crash.

## Settings and authentication

`settings.dpapi`: JSON encrypted with Windows DPAPI machine scope, UI forbidden.
Temporary ciphertext `settings.tmp` renamed over settings. State directory
protected DACL: SYSTEM, Administrators, service virtual SID only, inherited by
children. Authorized GUI user has **no state-file access**. Machine DPAPI alone
does not isolate local users; state-file ACL provides isolation. Administrators
remain trusted and can read/change state. Settings never include arbitrary paths,
commands, private-destination overrides or CA files.

Executable directory allows SYSTEM/Administrators full access, Users read/execute,
not write. ProgramFiles installation prevents authorized user replacing service.
Corrupt settings fail startup, never silently replaced or downgraded to insecure.

Named pipe: `\\.\pipe\HomeProxyAgent.v1`, byte stream. Windows DACL authenticates
SYSTEM, elevated Administrators, service SID and **one configured user SID**.
User pipe rights explicitly exclude FILE_CREATE_PIPE_INSTANCE. No Everyone,
Authenticated Users or Users grant. go-winio rejects remote clients and reserves
first instance; pipe collision fails startup. Other local users denied by OS.
Authorized user and administrators may perform all listed configuration/tunnel
operations. Pipe never installs/uninstalls, edits owner SID, controls SCM, reads
token back, runs commands or changes ACLs. No client-supplied SID/auth flag.

## IPC v1

One UTF-8 JSON request per new pipe connection, terminated by newline. One JSON
response + newline then server closes. Max request 16 KiB, 5-second connection
deadline; serial processing. Unknown JSON fields, unsupported versions and
unknown operations rejected. Flutter polls `status` every 1-2 seconds; no push
events/log stream. Open pipe via native Windows bridge (CreateFile read/write),
not TCP/HTTP. Use specific access mask `0x12019b`, not GENERIC_WRITE (its mapping
includes FILE_CREATE_PIPE_INSTANCE and is deliberately denied for owner).
Handle ERROR_FILE_NOT_FOUND (service stopped/not installed),
ERROR_ACCESS_DENIED (wrong authorized user), ERROR_PIPE_BUSY with bounded retry.
Verify server process via GetNamedPipeServerProcessId and SCM QueryServiceStatusEx
before sending secrets; require matching running service PID to reject local
pipe impersonation when service is stopped. Never log requests or tokens.

Operations:

| op | request fields | semantics |
| --- | --- | --- |
| `status` | version, op | live tunnel status + redacted config, if configured |
| `get_config` | version, op | same response as status |
| `set_config` | version, op, config | validate, save encrypted settings, replace tunnel if enabled |
| `connect` | version, op | persist enabled=true; restart existing configured tunnel |
| `disconnect` | version, op | persist enabled=false; cancel tunnel, keep service/IPC running |

```json
{"version":1,"op":"set_config","config":{"address":"example.com:4433","id":"ev-pc","token":"<at least 16 bytes>","fingerprint":"<64 hex SHA-256 leaf pin>","insecure":false,"enabled":true}}
```

Config fields: address host:port (port 1-65535); id 1-64 bytes, no NUL/newline or
slashes; token 16-4096 bytes; fingerprint exactly 64 hex characters (no colons)
unless insecure=true; enabled boolean; insecure boolean default **false**.
Missing/empty token on subsequent set_config retains stored token. Initial setup
requires token. Full config replacement, not patch; always send boolean values.

Secure default uses independently obtained SHA-256 server leaf pin, TLS 1.3.
GUI includes explicit insecure checkbox matching Android, unchecked by default.
Before accepting insecure=true, show warning: **Server identity is not verified;
an attacker may intercept authentication and traffic. Prefer trusted SHA-256 pin.**
Do not auto-enable on failure or infer from empty pin. Private destinations stay
blocked in both modes. Pin obtained from trusted server/admin channel only.

Success:

```json
{"version":1,"ok":true,"config":{"address":"example.com:4433","id":"ev-pc","fingerprint":"...","insecure":false,"enabled":true},"status":"Bağlı"}
```

No token key in response. Unconfigured success has no config and stopped status.
Status currently existing Turkish core strings: `Durduruldu`, `Bağlanıyor`,
`Bağlı`, `Bağlantı kesildi; 3 saniye sonra yeniden denenecek`. Treat unknown text
as displayable state, not fatal. `enabled` is persisted intent, status live state.
Error: `{"version":1,"ok":false,"error":"configure agent first"}`.
Save failures leave running tunnel unchanged. Start failure after save leaves
saved intent but returns error; user must retry/connect after resolving duplicate.

## Validation ceiling

Config/SDDL rejection and DPAPI roundtrip/replacement/corruption tests run without
service installation. Full Windows SCM/UAC/virtual-account/ACL integration,
cross-user denial, boot/crash/manual-stop recovery and packaged Flutter runtime
require separately approved installation in disposable Windows VM. Existing
Windows Unix-management test has previously timed out; record actual current
test result rather than treating historical timeout as new backend failure.

## Completed GUI validation

Flutter Windows release build passed. GUI launched without installed service;
rendered missing-service state was inspected. `flutter analyze` and 13 widget
tests passed, including Android restore and Windows consent/error/IPC commands.
Final `go test ./... -timeout 30s` passed (proxy 23.124s, windowsagent 2.425s).
Targeted Go vet and packaged Go helper build passed. One earlier combined
validation shell exceeded its 90-second limit; subsequent separate checks passed.

Local runnable package: `app/build/windows/x64/runner/Release/`. Go
`homeproxy.exe` copied beside `homeproxy-gui.exe`. No actual SCM installation,
boot/crash recovery or real service-account pipe connection tested yet.
