package com.homeproxy.homeproxy_agent

import android.content.Intent
import android.os.Build
import io.flutter.embedding.android.FlutterActivity
import io.flutter.embedding.engine.FlutterEngine
import io.flutter.plugin.common.MethodChannel

class MainActivity : FlutterActivity() {
    override fun configureFlutterEngine(engine: FlutterEngine) {
        super.configureFlutterEngine(engine)
        MethodChannel(engine.dartExecutor.binaryMessenger, "homeproxy/agent").setMethodCallHandler { call, result ->
            // Main-thread service state: never save replacement settings or claim a
            // new start while the current tunnel still owns its original settings.
            if ((call.method == "saveSettings" || call.method == "start") && AgentService.running) {
                result.error("RUNNING", "Ayarları değiştirmek veya yeniden başlatmak için önce bağlantıyı durdurun.", null)
                return@setMethodCallHandler
            }
            when (call.method) {
                "loadSettings" -> {
                    try { result.success(AgentSettings(this).load()) }
                    catch (e: Exception) { result.error("LOAD", "Kayıtlı ayarlar okunamadı. Kayıt silinmedi; bilgileri yeniden girip kaydedebilirsiniz.", null) }
                }
                "saveSettings" -> {
                    try {
                        AgentSettings(this).save(call.arguments as? Map<String, Any?> ?: emptyMap())
                        result.success(null)
                    } catch (e: Exception) { result.error("SAVE", "Ayarlar güvenli kaydedilemedi", null) }
                }
                "status" -> result.success(AgentService.status)
                "start" -> {
                    val intent = Intent(this, AgentService::class.java)
                    intent.putExtra("insecure", call.argument<Boolean>("insecure") ?: false)
                    for (key in listOf("address", "id", "token", "fingerprint")) {
                        intent.putExtra(key, call.argument<String>(key) ?: "")
                    }
                    try {
                        if (Build.VERSION.SDK_INT >= 33 && checkSelfPermission(android.Manifest.permission.POST_NOTIFICATIONS) != android.content.pm.PackageManager.PERMISSION_GRANTED) {
                            requestPermissions(arrayOf(android.Manifest.permission.POST_NOTIFICATIONS), 1)
                        }
                        val ticket = AgentService.reserveStart() ?: error("Agent already running")
                        intent.putExtra("generation", ticket)
                        try { startForegroundService(intent) }
                        catch (e: Exception) { AgentService.cancelStart(ticket); throw e }
                        result.success(null)
                    } catch (e: Exception) { result.error("START", "Arka plan servisi başlatılamadı", null) }
                }
                "stop" -> {
                    AgentService.cancelPendingStart()
                    stopService(Intent(this, AgentService::class.java))
                    result.success(null)
                }
                else -> result.notImplemented()
            }
        }
    }
}
