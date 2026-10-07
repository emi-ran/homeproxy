# Android active settings review

- HEAD `230ae8c` already serializes old service teardown and replacement startup. SDK-free restart and tile checks pass.
- Confirmed separate defect: `AgentService.onStartCommand` ignores start while `started`; `MainActivity` reports success. Flutter saves replacement settings before requesting start. Saved settings can therefore differ from active tunnel settings.
- Minimal correction: reject app save/start while service is running, with explicit stop-first error. Keep native tile/notification lifecycle unchanged. Successful start means request accepted, not QUIC authentication.
- SDK-free Kotlin tests can exercise method-channel rejection and preservation of settings; they do not prove device runtime or installed APK revision.
- Production phone `telefon` offline / immediate SOCKS REP=1 remains unresolved. Need installed APK revision, device logs, server membership and port-route correlation. No routing/keepalive change justified.
- Repository Compose publishes QUIC only; live public SOCKS is deployment drift, not grounds for source firewall or localhost bind changes. Need approved live inspection/isolation preserving Mori access.
- No APK build, deployment, restart or listener changes authorized. User-stopped HomeProxy must stay stopped.
