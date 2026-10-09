package com.homeproxy.homeproxy_agent

import android.app.*
import android.content.Intent
import android.os.Handler
import android.os.IBinder
import android.os.Looper
import mobile.Mobile
import mobile.StatusListener
import java.util.concurrent.Executors

class AgentService : Service() {
    companion object {
        @Volatile var status = "Durduruldu"
            private set
        @Volatile var running = false
            private set
        private var generation = 0L
        private var pending = false
        fun reserveStart(): Long? {
            if (running) return null
            generation++
            pending = true
            running = true
            status = "Bağlanıyor"
            return generation
        }
        fun cancelStart(ticket: Long) {
            if (pending && generation == ticket) {
                pending = false
                running = false
                status = "Durduruldu"
            }
        }
        fun cancelPendingStart() { cancelStart(generation) }
        // Process owner: replacement services must wait for old startup AND stop.
        private val worker = Executors.newSingleThreadExecutor()
    }
    private val agent = Mobile.newAgent()
    private val handler = Handler(Looper.getMainLooper())
    private var ownerGeneration = 0L
    private var started = false
    private var failed = false
    private var destroyed = false

    private fun publish(next: String) {
        status = next
        AgentTileService.refresh(this)
        if (started && !destroyed) getSystemService(NotificationManager::class.java).notify(1, notification())
    }

    private fun notification(): Notification {
        val open = PendingIntent.getActivity(this, 0, Intent(this, MainActivity::class.java), PendingIntent.FLAG_IMMUTABLE)
        val stop = PendingIntent.getService(this, 1, Intent(this, AgentService::class.java).setAction("stop"), PendingIntent.FLAG_IMMUTABLE)
        return Notification.Builder(this, "tunnel").setContentTitle("HomeProxy · Telefon çıkışı")
            .setContentText(status).setSmallIcon(R.drawable.ic_tunnel).setOnlyAlertOnce(true)
            .setContentIntent(open).setOngoing(true).addAction(Notification.Action.Builder(null, "Durdur", stop).build()).build()
    }

    override fun onCreate() {
        super.onCreate()
        getSystemService(NotificationManager::class.java).createNotificationChannel(NotificationChannel("tunnel", "Proxy bağlantısı", NotificationManager.IMPORTANCE_LOW))
        agent.setStatusListener(object : StatusListener {
            override fun onStatus(next: String) {
                handler.post { if (!destroyed && next != "Durduruldu") publish(next) }
            }
        })
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        if (intent?.action == "stop") { stopSelf(); return START_NOT_STICKY }
        // No automatic reconnect after process death; null restart has no credentials.
        if (intent == null) { stopSelf(); return START_NOT_STICKY }
        if (started) return START_NOT_STICKY
        val ticket = intent.getLongExtra("generation", 0L)
        if (ticket != 0L && (!pending || ticket != generation)) {
            stopSelf()
            return START_NOT_STICKY
        }
        if (ticket == 0L) generation++
        ownerGeneration = generation
        pending = false
        running = true
        publish("Bağlanıyor")
        startForeground(1, notification())
        started = true
        worker.execute {
            try {
                agent.startWithTLS(intent.getStringExtra("address") ?: "", intent.getStringExtra("id") ?: "",
                    intent.getStringExtra("token") ?: "", intent.getStringExtra("fingerprint") ?: "", intent.getBooleanExtra("insecure", false))
            } catch (e: Exception) {
                // Go validation errors contain no token or connection details.
                handler.post {
                    if (!destroyed) {
                        failed = true
                        publish(e.message ?: "Agent başlatılamadı")
                        stopSelf()
                    }
                }
            }
        }
        // ponytail: explicit user start only; reboot and process-death restart are not enabled.
        return START_NOT_STICKY
    }

    override fun onDestroy() {
        destroyed = true
        if (started && ownerGeneration == generation) publish("Durduruluyor")
        // Shared queue also holds replacement startup until this stop completes.
        worker.execute {
            agent.setStatusListener(null)
            agent.stop()
            handler.post {
                if (started && ownerGeneration == generation) {
                    running = false
                    publish(if (failed) "Agent başlatılamadı" else "Durduruldu")
                }
            }
        }
        // Process-lifetime worker: never shut it down from an individual service.
        super.onDestroy()
    }
    override fun onBind(intent: Intent?): IBinder? = null
}
