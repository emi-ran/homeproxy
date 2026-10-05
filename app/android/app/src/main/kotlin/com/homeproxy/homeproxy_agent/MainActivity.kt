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
            when (call.method) {
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
                        startForegroundService(intent)
                        result.success(null)
                    } catch (e: Exception) { result.error("START", "Arka plan servisi başlatılamadı", null) }
                }
                "stop" -> {
                    stopService(Intent(this, AgentService::class.java))
                    AgentService.status = "Durduruldu"
                    result.success(null)
                }
                else -> result.notImplemented()
            }
        }
    }
}
