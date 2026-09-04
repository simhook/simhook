package dev.simhook.app.ui

import android.Manifest
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.net.Uri
import android.os.Build
import android.os.PowerManager
import android.provider.Settings
import androidx.core.content.ContextCompat

/** Which runtime permissions the gateway needs, and why. */
object Permissions {
    data class Item(val permission: String, val title: String, val reason: String, val required: Boolean)

    val items: List<Item> = buildList {
        add(Item(Manifest.permission.SEND_SMS, "Send SMS", "Send the messages your API asks for.", required = true))
        add(Item(Manifest.permission.RECEIVE_SMS, "Receive SMS", "Forward incoming messages to your webhooks.", required = true))
        add(Item(Manifest.permission.READ_PHONE_STATE, "Phone state", "See which SIMs are in the phone so you can choose one.", required = false))
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
            add(Item(Manifest.permission.POST_NOTIFICATIONS, "Notifications", "Show sending progress and warn you if the pairing is lost.", required = false))
        }
    }

    fun granted(context: Context, permission: String): Boolean =
        ContextCompat.checkSelfPermission(context, permission) == PackageManager.PERMISSION_GRANTED

    fun coreGranted(context: Context): Boolean = items.filter { it.required }.all { granted(context, it.permission) }

    fun all(): Array<String> = items.map { it.permission }.toTypedArray()

    fun ignoringBatteryOptimizations(context: Context): Boolean =
        context.getSystemService(PowerManager::class.java)?.isIgnoringBatteryOptimizations(context.packageName) ?: true

    fun requestIgnoreBatteryOptimizations(context: Context) {
        runCatching {
            context.startActivity(
                Intent(Settings.ACTION_REQUEST_IGNORE_BATTERY_OPTIMIZATIONS)
                    .setData(Uri.parse("package:${context.packageName}"))
                    .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK),
            )
        }
    }

    fun openAppSettings(context: Context) {
        runCatching {
            context.startActivity(
                Intent(Settings.ACTION_APPLICATION_DETAILS_SETTINGS)
                    .setData(Uri.parse("package:${context.packageName}"))
                    .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK),
            )
        }
    }
}
