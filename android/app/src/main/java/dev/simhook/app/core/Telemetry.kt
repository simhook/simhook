package dev.simhook.app.core

import android.content.Context
import android.content.Intent
import android.content.IntentFilter
import android.net.ConnectivityManager
import android.net.NetworkCapabilities
import android.os.BatteryManager
import android.os.StatFs
import android.os.SystemClock
import dev.simhook.app.api.Telemetry
import java.util.Locale
import java.util.TimeZone

/** Snapshot of the phone's state for a heartbeat. Cheap and best-effort. */
object TelemetryCollector {
    fun collect(context: Context, keepAlive: Boolean, outboxPending: Int): Telemetry {
        var battery: Int? = null
        var charging: Boolean? = null
        runCatching {
            val status = context.registerReceiver(null, IntentFilter(Intent.ACTION_BATTERY_CHANGED))
            if (status != null) {
                val level = status.getIntExtra(BatteryManager.EXTRA_LEVEL, -1)
                val scale = status.getIntExtra(BatteryManager.EXTRA_SCALE, -1)
                if (level >= 0 && scale > 0) battery = (level * 100) / scale
                val st = status.getIntExtra(BatteryManager.EXTRA_STATUS, -1)
                charging = st == BatteryManager.BATTERY_STATUS_CHARGING || st == BatteryManager.BATTERY_STATUS_FULL
            }
        }
        val network = runCatching {
            val cm = context.getSystemService(ConnectivityManager::class.java)
            val caps = cm?.getNetworkCapabilities(cm.activeNetwork)
            when {
                caps == null -> "none"
                caps.hasTransport(NetworkCapabilities.TRANSPORT_WIFI) -> "wifi"
                caps.hasTransport(NetworkCapabilities.TRANSPORT_CELLULAR) -> "cellular"
                caps.hasTransport(NetworkCapabilities.TRANSPORT_ETHERNET) -> "ethernet"
                else -> "other"
            }
        }.getOrNull()
        val storageFree = runCatching { StatFs(context.filesDir.path).availableBytes }.getOrNull()
        return Telemetry(
            batteryPercent = battery,
            charging = charging,
            network = network,
            uptimeMs = SystemClock.elapsedRealtime(),
            timezone = TimeZone.getDefault().id,
            locale = Locale.getDefault().toLanguageTag(),
            storageFreeBytes = storageFree,
            keepAlive = keepAlive,
            outboxPending = outboxPending,
        )
    }
}
