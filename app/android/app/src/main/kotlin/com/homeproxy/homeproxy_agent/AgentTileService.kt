package com.homeproxy.homeproxy_agent

import android.app.PendingIntent
import android.content.ComponentName
import android.content.Context
import android.content.Intent
import android.os.Build
import android.service.quicksettings.Tile
import android.service.quicksettings.TileService
import android.widget.Toast

class AgentTileService : TileService() {
    companion object {
        private var listening: AgentTileService? = null
        fun refresh(context: Context) {
            if (Build.VERSION.SDK_INT < 24) return
            listening?.update()
            requestListeningState(context, ComponentName(context, AgentTileService::class.java))
        }
    }

    private fun update() {
        qsTile?.apply {
            label = "HomeProxy"
            state = if (AgentService.status == "Bağlı") Tile.STATE_ACTIVE else Tile.STATE_INACTIVE
            if (Build.VERSION.SDK_INT >= 29) subtitle = AgentService.status
            contentDescription = "HomeProxy · ${AgentService.status}"
            updateTile()
        }
    }

    override fun onStartListening() {
        super.onStartListening()
        listening = this
        update()
    }

    override fun onStopListening() {
        if (listening === this) listening = null
        super.onStopListening()
    }

    override fun onDestroy() {
        if (listening === this) listening = null
        super.onDestroy()
    }

    override fun onClick() {
        super.onClick()
        if (isLocked) unlockAndRun { toggle() } else toggle()
    }

    private fun toggle() {
        if (AgentService.running) {
            AgentService.cancelPendingStart()
            refresh(this)
            stopService(Intent(this, AgentService::class.java))
            return
        }
        try {
            val settings = AgentSettings(this).load()
            check(listOf("address", "id", "token").all { (settings[it] as? String)?.isNotBlank() == true })
            // Reuse saved, explicitly chosen TLS mode; never silently enable insecure.
            check(settings["insecure"] == true || (settings["fingerprint"] as? String)?.isNotBlank() == true)
            val intent = Intent(this, AgentService::class.java)
            for (key in listOf("address", "id", "token", "fingerprint")) intent.putExtra(key, settings[key] as? String ?: "")
            intent.putExtra("insecure", settings["insecure"] as? Boolean ?: false)
            val ticket = AgentService.reserveStart() ?: return
            intent.putExtra("generation", ticket)
            refresh(this)
            try { startForegroundService(intent) }
            catch (e: Exception) { AgentService.cancelStart(ticket); refresh(this); throw e }
        } catch (_: Exception) {
            Toast.makeText(this, "HomeProxy başlatılamadı. Uygulamadaki ayarları kontrol edin.", Toast.LENGTH_LONG).show()
            val open = Intent(this, MainActivity::class.java).addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
            if (Build.VERSION.SDK_INT >= 34) {
                startActivityAndCollapse(PendingIntent.getActivity(this, 2, open, PendingIntent.FLAG_IMMUTABLE or PendingIntent.FLAG_UPDATE_CURRENT))
            } else {
                @Suppress("DEPRECATION")
                startActivityAndCollapse(open)
            }
        }
    }
}
