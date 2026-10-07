# Android stop/restart review

At `8233a62`, each `AgentService` owns a separate startup executor. `onDestroy` clears the running flag before queued startup and stop finish. New service can authenticate first, then delayed old startup replaces same-ID server connection before old stop closes it.

SDK-free regression compiles real `AgentService.kt` with lifecycle/mobile stubs and gates old startup with a latch. Before fix, observed `[connect:2, connect:1, stop:1]`; required `[connect:1, stop:1, connect:2]`. Android lifecycle and gomobile transport are not exercised by these stubs.

Use one process-lifetime serial executor across service instances; keep per-service agents/listeners to prevent stale callbacks binding to replacement service. Do not shut shared executor down on service destruction. New startup waits for old teardown even though UI running flag clears immediately.

Notification channels, channel-aware notification builder and foreground-service starts require API 26. Explicit app minimum must be 26, rather than unresolved Flutter default. Android versions below 8.0 become unsupported.
