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
open class Intent {
    constructor()
    constructor(context: Any, target: Class<*>)
    var action: String? = null
    fun setAction(value: String): Intent { action = value; return this }
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
open class Service {
    open fun onCreate() {}
    open fun onStartCommand(intent: Intent?, flags: Int, startId: Int) = 0
    open fun onDestroy() {}
    open fun onBind(intent: Intent?): IBinder? = null
    fun stopSelf() {}
    fun startForeground(id: Int, notification: Notification) {}
    fun <T> getSystemService(type: Class<T>): T = type.getDeclaredConstructor().newInstance()
    companion object { const val START_NOT_STICKY = 2 }
}
class PendingIntent { companion object {
    const val FLAG_IMMUTABLE = 1
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
class MainActivity
object R { object drawable { const val ic_tunnel = 1 } }
object AgentTileService { fun refresh(context: Any) {} }
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
    val old = AgentService()
    old.onCreate()
    old.onStartCommand(Intent(), 0, 1)
    check(AgentService.running && AgentService.status == "Bağlanıyor")
    check(Mobile.entered.await(5, TimeUnit.SECONDS))
    val oldWorker = worker(old)
    old.onDestroy()
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
    replacement.onDestroy()
    if (!newWorker.isShutdown) newWorker.submit {}.get(5, TimeUnit.SECONDS)
    else check(newWorker.awaitTermination(5, TimeUnit.SECONDS))
    Handler.drain()
    check(!AgentService.running && AgentService.status == "Durduruldu")
    check(Mobile.events.toList() == listOf("connect:1", "stop:1", "connect:2", "stop:2"))
    println("PASS: delayed old startup, serialized teardown/restart, stale callback guard, final stop")
    kotlin.system.exitProcess(0)
}
''',
}


def main():
    scratch = Path(os.environ.get('TMPDIR', str(Path.home() / '.hermes/cache/scratch')))
    scratch.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix='homeproxy-restart-', dir=scratch) as tmp:
        work = Path(tmp)
        for name, content in STUBS.items():
            (work / name).write_text(content)
        sources = [str(SERVICE), *(str(work / name) for name in STUBS)]
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
