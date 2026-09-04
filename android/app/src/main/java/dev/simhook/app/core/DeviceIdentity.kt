package dev.simhook.app.core

import android.annotation.SuppressLint
import android.content.Context
import android.os.Build
import android.provider.Settings
import java.security.MessageDigest

/** Facts about this handset that the server records at pairing. */
object DeviceIdentity {
    /**
     * A stable, opaque identifier for this phone on this account, so
     * re-pairing after a reinstall lands on the same device record instead of
     * creating a duplicate. It never leaves the phone in the clear.
     */
    @SuppressLint("HardwareIds")
    fun hardwareKey(context: Context): String {
        val androidId = Settings.Secure.getString(context.contentResolver, Settings.Secure.ANDROID_ID) ?: "unknown"
        val raw = "$androidId|${Build.MANUFACTURER}|${Build.MODEL}|${Build.DEVICE}"
        val digest = MessageDigest.getInstance("SHA-256").digest(raw.toByteArray(Charsets.UTF_8))
        return digest.joinToString("") { "%02x".format(it) }
    }

    val manufacturer: String get() = Build.MANUFACTURER
    val brand: String get() = Build.BRAND
    val model: String get() = Build.MODEL
    val buildId: String get() = Build.ID
    val osVersion: String get() = Build.VERSION.RELEASE
    val osApiLevel: Int get() = Build.VERSION.SDK_INT

    fun defaultName(): String {
        val brand = Build.BRAND.replaceFirstChar { it.uppercase() }
        return if (Build.MODEL.startsWith(Build.BRAND, ignoreCase = true)) Build.MODEL else "$brand ${Build.MODEL}"
    }
}
