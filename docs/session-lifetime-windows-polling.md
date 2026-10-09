# Session lifetime and Windows polling investigation

Baseline: 4f4b9a0. TCP bridges and both ends of UDP associations impose a one-hour hard deadline. This is not a sliding idle timeout; UDP has an independent idle policy. Increasing only one endpoint cannot extend a session beyond the other endpoint's cap. Existing sessions remain pinned and must close on expiry; no seamless migration is promised.

Windows Flutter polls local SCM then authenticated named-pipe status every two seconds and always calls setState, even unchanged. This is local IPC/CPU work, not tunnel bandwidth. Preserve native SCM/pipe peer validation. Suspend automatic refresh when not resumed, discard stale in-flight replies, refresh on resume, and avoid unchanged screen rebuilds.

Scope: bounded per-run lifetime configuration without wire changes, one-hour default; explicit endpoint settings, not negotiation. No Android UI/native changes, MTU changes, Windows/Android builds, live service changes or deployment.
