package dev.simhook.app.sms

import android.Manifest
import android.content.Context
import android.content.pm.PackageManager
import android.telephony.SubscriptionManager
import androidx.core.content.ContextCompat
import dev.simhook.app.api.SimDescriptor

/** Reads the SIMs in the phone. Needs READ_PHONE_STATE. */
object SimInfo {
    fun hasPermission(context: Context): Boolean =
        ContextCompat.checkSelfPermission(context, Manifest.permission.READ_PHONE_STATE) == PackageManager.PERMISSION_GRANTED

    fun list(context: Context): List<SimDescriptor> {
        if (!hasPermission(context)) return emptyList()
        return try {
            val manager = context.getSystemService(SubscriptionManager::class.java) ?: return emptyList()
            @Suppress("MissingPermission")
            val subs = manager.activeSubscriptionInfoList ?: return emptyList()
            subs.map {
                SimDescriptor(
                    subscriptionId = it.subscriptionId,
                    slot = it.simSlotIndex,
                    carrier = it.carrierName?.toString()?.takeIf { s -> s.isNotBlank() },
                    displayName = it.displayName?.toString()?.takeIf { s -> s.isNotBlank() },
                    country = it.countryIso?.takeIf { s -> s.isNotBlank() },
                )
            }.sortedBy { it.slot }
        } catch (e: SecurityException) {
            emptyList()
        }
    }

    fun isValidSubscription(context: Context, subscriptionId: Int): Boolean =
        list(context).any { it.subscriptionId == subscriptionId }
}
