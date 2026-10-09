#!/usr/bin/env python3
"""Run real AgentService Kotlin against SDK-free lifecycle/mobile stubs.

No Gradle, Android SDK, AAR or device involved. Set KOTLIN_LIB to compiler
lib directory if kotlinc is absent and cached Kotlin compiler is unavailable.
"""
import os
from pathlib import Path
import shutil
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parents[1]
SERVICE = ROOT / 'app/android/app/src/main/kotlin/com/homeproxy/homeproxy_agent/AgentService.kt'
STUBS = {
    'content.kt': '''package android.content
open class Context {
    var starts = 0
    var stops = 0
    var failStart = false
    var dispatched: Intent? = null
    fun startForegroundService(intent: Intent) {
        if (failStart) error("dispatch denied")
        starts++
        dispatched = intent
        android.app.Lifecycle.enqueue(intent)
    }
    fun stopService(intent: Intent) { stops++; android.app.Lifecycle.stop() }
}
class ComponentName(context: Context, type: Class<*>)
open class Intent {
    constructor()
    constructor(context: Any, target: Class<*>)
    var action: String? = null
    fun setAction(value: String): Intent { action = value; return this }
    private val extras = mutableMapOf<String, Any>()
    fun putExtra(key: String, value: Any): Intent { extras[key] = value; return this }
    fun getLongExtra(key: String, default: Long): Long = extras[key] as? Long ?: default
    fun addFlags(flags: Int): Intent = this
    companion object { const val FLAG_ACTIVITY_NEW_TASK = 1 }
    fun getStringExtra(key: String): String? = null
    fun getBooleanExtra(key: String, default: Boolean): Boolean = default
}
''',
    'os.kt': '''package android.os
interface IBinder
class Looper { companion object { fun getMainLooper() = Looper() } }
class Handler(looper: Looper) {
    fun post(task: () -> Unit) { synchronized(tasks) { tasks.add(task) } }
    companion object {
        private val tasks = mutableListOf<() -> Unit>()
        fun drain() { while (true) {
            val task = synchronized(tasks) { if (tasks.isEmpty()) null else tasks.removeAt(0) } ?: return
            task()
        } }
    }
}
''',
    'app.kt': '''package android.app
import android.content.Intent
import android.os.IBinder
open class Service : android.content.Context() {
    open fun onCreate() {}
    open fun onStartCommand(intent: Intent?, flags: Int, startId: Int) = 0
    open fun onDestroy() {}
    open fun onBind(intent: Intent?): IBinder? = null
    fun stopSelf() { Lifecycle.stop(this) }
    fun stopSelf(startId: Int) { Lifecycle.stop(this, startId) }
    fun startForeground(id: Int, notification: Notification) {}
    fun <T> getSystemService(type: Class<T>): T = type.getDeclaredConstructor().newInstance()
    companion object { const val START_NOT_STICKY = 2 }
}

// Deterministic started-service model: queued intents have assigned start IDs.
object Lifecycle {
    val queue = mutableListOf<Pair<Int, Intent>>()
    var current: Service? = null
    var latest = 0
    fun enqueue(intent: Intent) { queue.add(++latest to intent) }
    fun deliver(): Service {
        val (id, intent) = queue.removeAt(0)
        val service = current ?: com.homeproxy.homeproxy_agent.AgentService().also {
            current = it; it.onCreate()
        }
        service.onStartCommand(intent, 0, id)
        return service
    }
    fun stop(service: Service? = current, id: Int? = null) {
        if (service != null && service === current && (id == null || id == latest)) {
            current = null
            service.onDestroy()
        }
    }
}
class PendingIntent { companion object {
    const val FLAG_IMMUTABLE = 1
    const val FLAG_UPDATE_CURRENT = 2
    fun getActivity(context: Any, code: Int, intent: Intent, flags: Int) = PendingIntent()
    fun getService(context: Any, code: Int, intent: Intent, flags: Int) = PendingIntent()
} }
class Notification {
    class Builder(context: Any, channel: String) {
        fun setContentTitle(value: String) = this
        fun setContentText(value: String) = this
        fun setSmallIcon(value: Int) = this
        fun setOnlyAlertOnce(value: Boolean) = this
        fun setContentIntent(value: PendingIntent) = this
        fun setOngoing(value: Boolean) = this
        fun addAction(value: Action) = this
        fun build() = Notification()
    }
    class Action { class Builder(icon: Any?, title: String, intent: PendingIntent) {
        fun build() = Action()
    } }
}
class NotificationManager {
    fun notify(id: Int, notification: Notification) {}
    fun createNotificationChannel(channel: NotificationChannel) {}
    companion object { const val IMPORTANCE_LOW = 1 }
}
class NotificationChannel(id: String, title: String, importance: Int)
''',
    'mobile.kt': '''package mobile
import java.util.concurrent.CountDownLatch
import java.util.concurrent.TimeUnit
interface StatusListener { fun onStatus(next: String) }
object Mobile {
    val events = java.util.Collections.synchronizedList(mutableListOf<String>())
    val entered = CountDownLatch(1)
    val release = CountDownLatch(1)
    val stopEntered = CountDownLatch(1)
    val stopRelease = CountDownLatch(1)
    var count = 0
    fun newAgent() = Agent(++count)
}
class Agent(val id: Int) {
    var listener: StatusListener? = null
    fun setStatusListener(value: StatusListener?) { listener = value }
    fun startWithTLS(address: String, name: String, token: String, fingerprint: String, insecure: Boolean) {
        if (id == 1) {
            Mobile.entered.countDown()
            check(Mobile.release.await(5, TimeUnit.SECONDS)) { "startup gate timed out" }
        }
        Mobile.events.add("connect:$id")
        listener?.onStatus("Bağlı")
    }
    fun stop() {
        if (id == 1) {
            Mobile.stopEntered.countDown()
            check(Mobile.stopRelease.await(5, TimeUnit.SECONDS)) { "stop gate timed out" }
        }
        Mobile.events.add("stop:$id")
    }
}
''',
    'check.kt': '''package com.homeproxy.homeproxy_agent
import android.content.Intent
import android.os.Handler
import mobile.Mobile
import java.util.concurrent.ExecutorService
import java.util.concurrent.TimeUnit
object R { object drawable { const val ic_tunnel = 1 } }

fun worker(service: AgentService): ExecutorService {
    val field = AgentService::class.java.getDeclaredField("worker")
    field.isAccessible = true
    return field.get(service) as ExecutorService
}
fun main() {
    Thread.setDefaultUncaughtExceptionHandler { _, error ->
        error.printStackTrace()
        kotlin.system.exitProcess(1)
    }
    val activity = MainActivity()
    activity.configureFlutterEngine(io.flutter.embedding.engine.FlutterEngine())
    activity.failStart = true
    val rejected = ChannelResult()
    io.flutter.plugin.common.MethodChannel.handler(
        io.flutter.plugin.common.MethodCall("start", emptyMap<String, Any?>()), rejected)
    check(rejected.code == "START" && !AgentService.running && AgentService.status == "Durduruldu") {
        "failed dispatch leaked start reservation"
    }
    activity.failStart = false
    val first = ChannelResult()
    io.flutter.plugin.common.MethodChannel.handler(
        io.flutter.plugin.common.MethodCall("start", emptyMap<String, Any?>()), first)
    check(first.succeeded && activity.starts == 1)
    check(AgentService.running && AgentService.status == "Bağlanıyor") {
        "accepted start was not reserved before service dispatch"
    }
    rejectActiveSettings()
    // A was queued but canceled before Android created the service. B is accepted.
    AgentService.cancelPendingStart()
    activity.configureFlutterEngine(io.flutter.embedding.engine.FlutterEngine())
    val second = ChannelResult()
    io.flutter.plugin.common.MethodChannel.handler(
        io.flutter.plugin.common.MethodCall("start", emptyMap<String, Any?>()), second)
    check(second.succeeded)
    val ticketB = activity.dispatched!!.getLongExtra("generation", 0)
    check(ticketB != 0L) { "app dispatched zero-generation shortcut" }
    val staleA = android.app.Lifecycle.deliver()
    check(android.app.Lifecycle.current === staleA) {
        "stale A stopped service while accepted B was queued"
    }
    check(AgentService.running) { "stale A cleared B reservation" }
    val old = android.app.Lifecycle.deliver() as AgentService
    check(old === staleA) { "B delivered to destroyed instance" }
    check(activity.dispatched!!.getLongExtra("generation", 0) == ticketB)
    check(AgentService.running && AgentService.status == "Bağlanıyor")
    check(Mobile.entered.await(5, TimeUnit.SECONDS))
    rejectActiveSettings()
    val oldWorker = worker(old)
    android.app.Lifecycle.stop(old)
    check(AgentService.running && AgentService.status == "Durduruluyor") {
        "service claimed stopped before queued Go teardown completed"
    }
    rejectActiveSettings()
    val replacement = AgentService()
    replacement.onCreate()
    replacement.onStartCommand(Intent(), 0, 2)
    val newWorker = worker(replacement)
    // Drain independent replacement executor before releasing old startup.
    // Shared executor cannot drain until old startup and teardown finish.
    if (oldWorker !== newWorker) newWorker.submit {}.get(5, TimeUnit.SECONDS)
    Mobile.release.countDown()
    check(Mobile.stopEntered.await(5, TimeUnit.SECONDS))
    check(Mobile.events.toList() == listOf("connect:1")) {
        "replacement connected before old stop finished: ${Mobile.events}"
    }
    Handler.drain()
    check(AgentService.status == "Bağlanıyor") { "destroyed callback changed replacement status" }
    Mobile.stopRelease.countDown()
    if (!oldWorker.isShutdown) oldWorker.submit {}.get(5, TimeUnit.SECONDS)
    else check(oldWorker.awaitTermination(5, TimeUnit.SECONDS))
    newWorker.submit {}.get(5, TimeUnit.SECONDS)
    check(Mobile.events.toList() == listOf("connect:1", "stop:1", "connect:2")) {
        "old startup replaced newer connection: ${Mobile.events}"
    }
    Handler.drain()
    check(AgentService.running && AgentService.status == "Bağlı")
    rejectActiveSettings()
    replacement.onDestroy()
    if (!newWorker.isShutdown) newWorker.submit {}.get(5, TimeUnit.SECONDS)
    else check(newWorker.awaitTermination(5, TimeUnit.SECONDS))
    Handler.drain()
    check(!AgentService.running && AgentService.status == "Durduruldu")
    check(Mobile.events.toList() == listOf("connect:1", "stop:1", "connect:2", "stop:2"))
    acceptStoppedSettings()
    val tile = AgentTileService()
    tile.onStartListening()
    check(tile.qsTile!!.state == android.service.quicksettings.Tile.STATE_INACTIVE)
    tile.onClick()
    check(tile.starts == 0 && tile.stops == 1) { "tile started a second pending run" }
    check(tile.qsTile!!.subtitle == "Durduruldu") { "pending stop left tile connecting" }
    AgentSettings.saved = mapOf("address" to "test:443", "id" to "original", "token" to "test", "fingerprint" to "test")
    tile.onClick()
    check(tile.starts == 1 && AgentService.running)
    rejectActiveSettings()
    check(tile.qsTile!!.state == android.service.quicksettings.Tile.STATE_INACTIVE)
    val canceled = tile.dispatched!!
    tile.onClick()
    val stale = AgentService()
    stale.onCreate()
    stale.onStartCommand(canceled, 0, 3)
    check(!AgentService.running && AgentService.status == "Durduruldu")
    stale.onDestroy()
    worker(stale).submit {}.get(5, TimeUnit.SECONDS)
    Handler.drain()
    check(Mobile.events.none { it == "connect:3" }) { "canceled request started Go" }
    AgentService.reserveStart()!!
    val unconsumed = AgentService()
    unconsumed.onCreate()
    unconsumed.onDestroy()
    worker(unconsumed).submit {}.get(5, TimeUnit.SECONDS)
    Handler.drain()
    check(!AgentService.running && AgentService.status == "Durduruldu") {
        "destruction before consumption stranded accepted generation"
    }
    val older = AgentService()
    AgentService.reserveStart()!!
    older.onCreate()
    AgentService.cancelPendingStart()
    val newer = AgentService.reserveStart()!!
    older.onDestroy()
    worker(older).submit {}.get(5, TimeUnit.SECONDS)
    Handler.drain()
    check(AgentService.running) { "old unstarted destruction canceled newer reservation" }
    AgentService.cancelStart(newer)
    println("PASS: stale queued delivery, unconsumed gate cleanup, delayed startup, serialized teardown/restart, settings and tile controls")
    kotlin.system.exitProcess(0)
}
''',
}


STUBS.update({
    'flutter.kt': '''package io.flutter.embedding.android
import android.content.Intent
import io.flutter.embedding.engine.FlutterEngine
open class FlutterActivity : android.content.Context() {
    open fun configureFlutterEngine(engine: FlutterEngine) {}
    fun checkSelfPermission(permission: String) = 0
    fun requestPermissions(permissions: Array<String>, code: Int) {}

}
''',
    'engine.kt': '''package io.flutter.embedding.engine
class FlutterEngine { val dartExecutor = Executor() }
class Executor { val binaryMessenger = Any() }
''',
    'channel.kt': '''package io.flutter.plugin.common
class MethodCall(val method: String, val arguments: Any?) {
    fun <T> argument(key: String): T? = (arguments as? Map<String, Any?>)?.get(key) as? T
}
class MethodChannel(messenger: Any, name: String) {
    interface Result {
        fun success(value: Any?)
        fun error(code: String, message: String?, details: Any?)
        fun notImplemented()
    }
    fun setMethodCallHandler(value: (MethodCall, Result) -> Unit) { handler = value }
    companion object { lateinit var handler: (MethodCall, Result) -> Unit }
}
''',
    'platform.kt': '''package android
object Manifest { object permission { const val POST_NOTIFICATIONS = "notifications" } }
''',
    'permission.kt': '''package android.content.pm
object PackageManager { const val PERMISSION_GRANTED = 0 }
''',
    'settings.kt': '''package com.homeproxy.homeproxy_agent
class AgentSettings(context: Any) {
    fun load() = saved
    fun save(value: Map<String, Any?>) { saved = value }
    companion object { var saved: Map<String, Any?> = mapOf("id" to "original") }
}
class ChannelResult : io.flutter.plugin.common.MethodChannel.Result {
    var succeeded = false
    var code: String? = null
    override fun success(value: Any?) { succeeded = true }
    override fun error(code: String, message: String?, details: Any?) { this.code = code }
    override fun notImplemented() { error("UNKNOWN", null, null) }
}
fun rejectActiveSettings() {
    val activity = MainActivity()
    activity.configureFlutterEngine(io.flutter.embedding.engine.FlutterEngine())
    for (method in listOf("saveSettings", "start")) {
        val result = ChannelResult()
        io.flutter.plugin.common.MethodChannel.handler(
            io.flutter.plugin.common.MethodCall(method, mapOf("id" to "replacement")), result)
        check(!result.succeeded && result.code == "RUNNING") { "$method falsely accepted while running" }
    }
    check(AgentSettings.saved["id"] == "original") { "active settings overwritten" }
    check(activity.starts == 0) { "replacement start dispatched" }
}
fun acceptStoppedSettings() {
    val activity = MainActivity()
    activity.configureFlutterEngine(io.flutter.embedding.engine.FlutterEngine())
    for (method in listOf("saveSettings", "start")) {
        val result = ChannelResult()
        io.flutter.plugin.common.MethodChannel.handler(
            io.flutter.plugin.common.MethodCall(method, mapOf("id" to "replacement")), result)
        check(result.succeeded && result.code == null) { "$method rejected while stopped" }
    }
    check(AgentSettings.saved["id"] == "replacement")
    check(activity.starts == 1)
}
''',
})
STUBS.update({
    'tile.kt': """package android.service.quicksettings
class Tile {
    var label = ""
    var state = 0
    var subtitle = ""
    var contentDescription = ""
    fun updateTile() {}
    companion object { const val STATE_ACTIVE = 2; const val STATE_INACTIVE = 1 }
}
open class TileService : android.app.Service() {
    val qsTile: Tile? = Tile()
    val isLocked = false
    open fun onStartListening() {}
    open fun onStopListening() {}
    open fun onClick() {}
    fun unlockAndRun(task: () -> Unit) { task() }
    fun startActivityAndCollapse(intent: android.content.Intent) {}
    fun startActivityAndCollapse(intent: android.app.PendingIntent) {}
    companion object {
        fun requestListeningState(context: android.content.Context, name: android.content.ComponentName) {}
    }
}
""",
    'toast.kt': """package android.widget
class Toast {
    fun show() {}
    companion object {
        const val LENGTH_LONG = 1
        fun makeText(context: Any, text: String, length: Int) = Toast()
    }
}
""",
})
STUBS['os.kt'] += '\nobject Build { object VERSION { const val SDK_INT = 33 } }\n'


def main():
    scratch = Path(os.environ.get('TMPDIR', str(Path.home() / '.hermes/cache/scratch')))
    scratch.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix='homeproxy-restart-', dir=scratch) as tmp:
        work = Path(tmp)
        for name, content in STUBS.items():
            (work / name).write_text(content)
        sources = [str(SERVICE), str(SERVICE.with_name('MainActivity.kt')), str(SERVICE.with_name('AgentTileService.kt')), *(str(work / name) for name in STUBS)]
        kotlinc = shutil.which('kotlinc')
        if kotlinc:
            compiler = [kotlinc]
            # Use installed compiler lib for explicit runtime classpath.
            lib = Path(kotlinc).resolve().parents[1] / 'lib'
        else:
            configured = os.environ.get('KOTLIN_LIB')
            candidates = sorted((Path.home() / '.gradle/wrapper/dists').glob('*/**/lib/kotlin-compiler-embeddable-*.jar'))
            if not configured and not candidates:
                raise SystemExit('Kotlin compiler unavailable; set KOTLIN_LIB or install kotlinc')
            lib = Path(configured) if configured else candidates[-1].parent
            compiler = ['java', '-cp', str(lib / '*'), 'org.jetbrains.kotlin.cli.jvm.K2JVMCompiler']
        stdlib = next(lib.glob('kotlin-stdlib*.jar'))
        subprocess.run([*compiler, '-nowarn', '-no-stdlib', '-no-reflect', '-classpath', str(stdlib),
                        '-d', str(work / 'classes'), *sources], check=True)
        subprocess.run(['java', '-cp', os.pathsep.join([str(work / 'classes'), str(stdlib)]),
                        'com.homeproxy.homeproxy_agent.CheckKt'], check=True, timeout=20)


if __name__ == '__main__':
    main()
