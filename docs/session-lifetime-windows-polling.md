# Session lifetime and Windows polling investigation

Baseline: 4f4b9a0. TCP bridges and both ends of UDP associations impose a one-hour hard deadline. This is not a sliding idle timeout; UDP has an independent idle policy. Increasing only one endpoint cannot extend a session beyond the other endpoint's cap. Existing sessions remain pinned and must close on expiry; no seamless migration is promised.

Windows Flutter polls local SCM then authenticated named-pipe status every two seconds and always calls setState, even unchanged. This is local IPC/CPU work, not tunnel bandwidth. Preserve native SCM/pipe peer validation. Suspend automatic refresh when not resumed, discard stale in-flight replies, refresh on resume, and avoid unchanged screen rebuilds.

Implemented safe subset: CLI server and agent accept `-session-lifetime` (Go duration, >0 and <=24h; default 1h). Both TCP and UDP consume a per-run context value, never a mutable global. Shared Go MobileAgent has additive `StartWithTLSAndLifetime(..., seconds)` (1–86400); existing Start/StartWithTLS callers keep 3600 seconds. No wire change or negotiation: the shorter endpoint cap wins. Existing sessions are not migrated or updated.

Deferred: Windows persisted IPC Config and gomobile wrapper/UI lifetime controls remain unchanged at one hour. Extending only the server cannot lengthen those clients beyond one hour. This avoids changing existing redacted-config replacement and Android API contracts in this scope. No Android UI/native changes, MTU changes, Windows/Android builds, live service changes or deployment.

Windows widget RED: paused six-second pump polled SCM/pipe four times instead of once; unchanged-status identity assertion was false. GREEN: offscreen polling stops, immediate resume refresh and unchanged widget identity pass. Native peer validation is untouched. Go RED: CLI flag missing, TCP timed out rather than closed, UDP server/agent caps not enforced; GREEN: validated CLI options and real localhost TCP/UDP expiry tests pass. Shared MobileAgent rejects invalid/overflowing seconds before changing state.
