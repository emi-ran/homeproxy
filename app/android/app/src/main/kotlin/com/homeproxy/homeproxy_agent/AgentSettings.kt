package com.homeproxy.homeproxy_agent

import android.content.Context
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import android.util.Base64
import org.json.JSONObject
import java.security.KeyStore
import javax.crypto.Cipher
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import javax.crypto.spec.GCMParameterSpec

// Platform AES-GCM encrypts the whole settings record; key never leaves Android Keystore.
class AgentSettings(context: Context) {
    private val prefs = context.getSharedPreferences("agent_settings", Context.MODE_PRIVATE)
    private fun key(create: Boolean): SecretKey {
        val store = KeyStore.getInstance("AndroidKeyStore").apply { load(null) }
        (store.getKey("homeproxy_settings", null) as? SecretKey)?.let { return it }
        check(create) { "Kayıt anahtarı bulunamadı" }
        return KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, "AndroidKeyStore").apply {
            init(KeyGenParameterSpec.Builder("homeproxy_settings", KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT)
                .setBlockModes(KeyProperties.BLOCK_MODE_GCM).setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE).build())
        }.generateKey()
    }
    fun save(values: Map<String, Any?>) {
        val record = JSONObject()
        for (field in listOf("address", "id", "token", "fingerprint")) {
            val value = values[field] as? String ?: ""
            require(value.length <= 4096) { "Ayar çok uzun" }
            record.put(field, value)
        }
        record.put("insecure", values["insecure"] as? Boolean ?: false)
        val cipher = Cipher.getInstance("AES/GCM/NoPadding")
        cipher.init(Cipher.ENCRYPT_MODE, key(true))
        val encrypted = cipher.doFinal(record.toString().toByteArray(Charsets.UTF_8))
        check(prefs.edit().putString("record", Base64.encodeToString(cipher.iv + encrypted, Base64.NO_WRAP)).commit()) { "Ayar kaydedilemedi" }
    }
    fun load(): Map<String, Any> {
        val encoded = prefs.getString("record", null) ?: return emptyMap()
        val bytes = Base64.decode(encoded, Base64.NO_WRAP)
        require(bytes.size >= 28) { "Kayıt bozuk" }
        val cipher = Cipher.getInstance("AES/GCM/NoPadding")
        cipher.init(Cipher.DECRYPT_MODE, key(false), GCMParameterSpec(128, bytes.copyOfRange(0, 12)))
        val record = JSONObject(String(cipher.doFinal(bytes.copyOfRange(12, bytes.size)), Charsets.UTF_8))
        return mapOf("address" to record.getString("address"), "id" to record.getString("id"),
            "token" to record.getString("token"), "fingerprint" to record.getString("fingerprint"), "insecure" to record.getBoolean("insecure"))
    }
}
