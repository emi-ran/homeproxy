# Android active settings review

- HEAD `230ae8c` already serializes old service teardown and replacement startup. SDK-free restart and tile checks pass.
- Confirmed separate defect: `AgentService.onStartCommand` ignores start while `started`; `MainActivity` reports success. Flutter saves replacement settings before requesting start. Saved settings can therefore differ from active tunnel settings.
- Minimal correction: reject app save/start while service is running, with explicit stop-first error. Keep native tile/notification lifecycle unchanged. Successful start means request accepted, not QUIC authentication.
- SDK-free Kotlin tests can exercise method-channel rejection and preservation of settings; they do not prove device runtime or installed APK revision.
- Production phone `telefon` offline / immediate SOCKS REP=1 remains unresolved. Need installed APK revision, device logs, server membership and port-route correlation. No routing/keepalive change justified.
- Repository Compose publishes QUIC only; live public SOCKS is deployment drift, not grounds for source firewall or localhost bind changes. Need approved live inspection/isolation preserving Mori access.
- No APK build, deployment, restart or listener changes authorized. User-stopped HomeProxy must stay stopped.

## Source correction verification

- Added method-channel `RUNNING` error for save/start while service is connecting or connected; saved settings stay unchanged. User must stop first. No automatic restart or transport change.
- TDD RED: real `MainActivity.kt` in SDK-free harness failed with `saveSettings falsely accepted while running`. GREEN: connecting/connected rejection, saved-state preservation, no replacement dispatch, stopped save/start acceptance and existing serialized lifecycle checks passed.
- `python3 scripts/check_android_tile.py`: 4 tests passed. `go test -count=1 ./internal/proxy`: passed (22.053s); `go vet ./internal/proxy` and `git diff --check`: passed. Go commands used `GOMAXPROCS=2 GOFLAGS=-p=1`.
- Initial combined Go command included nonexistent `./internal/agent`; setup failed for that path. Corrected proxy command passed; no claimed agent-package test.
- Precommit self-review (independent delegation unavailable): no added secrets, unsafe evaluation, shell calls or network changes; method-channel guard runs on Android main thread. Tests compile real activity/service, but stub Android/Flutter/settings platform behavior. No device, APK, full race-suite or deployment proof.
- Guard applies once service is running; request acceptance before service callback is not authentication proof. Deployment exposure and phone offline cause remain unknown.
