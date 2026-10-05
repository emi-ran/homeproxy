package com.homeproxy.homeproxy_agent

import android.app.*
import android.content.Intent
import android.os.Handler
import android.os.IBinder
import android.os.Looper
import mobile.Mobile
import java.util.concurrent.Executors

class AgentService : Service() {
    companion object { @Volatile var status = "Durduruldu" }
    private val agent = Mobile.newAgent()
    private val worker = Executors.newSingleThreadExecutor()
    private val handler = Handler(Looper.getMainLooper())
    private var started = false
    private var failed = false
    private var destroyed = false
    private val refresh = object : Runnable {
        override fun run() {
            val next = agent.status()
            if (status != next) {
                status = next
                getSystemService(NotificationManager::class.java).notify(1, notification())
            }
            handler.postDelayed(this, 2000)
        }
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
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        if (intent?.action == "stop") { stopSelf(); return START_NOT_STICKY }
        if (started) return START_NOT_STICKY
        started = true
        status = "Bağlanıyor"
        startForeground(1, notification())
        worker.execute {
            try {
                agent.startWithTLS(intent?.getStringExtra("address") ?: "", intent?.getStringExtra("id") ?: "",
                    intent?.getStringExtra("token") ?: "", intent?.getStringExtra("fingerprint") ?: "", intent?.getBooleanExtra("insecure", false) ?: false)
                handler.post { if (!destroyed) { handler.removeCallbacks(refresh); handler.post(refresh) } }
            } catch (e: Exception) {
                // Go validation errors contain no token or connection details.
                status = e.message ?: "Agent başlatılamadı"
                handler.post { failed = true; stopSelf() }
            }
        }
        // ponytail: explicit user start only; reboot and process-death restart are not enabled.
        return START_NOT_STICKY
    }

    override fun onDestroy() {
        destroyed = true
        handler.removeCallbacks(refresh)
        if (!failed) status = "Durduruldu"
        worker.execute { agent.stop() }
        worker.shutdown()
        super.onDestroy()
    }
    override fun onBind(intent: Intent?): IBinder? = null
}
