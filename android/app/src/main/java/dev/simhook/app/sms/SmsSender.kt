package dev.simhook.app.sms

import android.Manifest
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.os.Build
import android.telephony.SmsManager
import androidx.core.content.ContextCompat
import kotlinx.coroutines.CompletableDeferred
import java.util.concurrent.ConcurrentHashMap

/**
 * Hands one message to the radio, in two steps: first the text is divided
 * into segments, so the outbox can record how many broadcasts to expect;
 * then the segments are given to the radio. Results arrive later through
 * [SmsStatusReceiver].
 */
object SmsSender {
    sealed interface Prepared {
        class Ready(val manager: SmsManager, val parts: ArrayList<String>) : Prepared
        class Failed(val failure: SmsFailure) : Prepared
    }

    sealed interface Outcome {
        /** The radio accepted [parts] segments; a sent broadcast follows for each. */
        data class Handed(val parts: Int) : Outcome
        data class Failed(val failure: SmsFailure) : Outcome
    }

    const val ACTION_SENT = "dev.simhook.app.SMS_SENT"
    const val ACTION_DELIVERED = "dev.simhook.app.SMS_DELIVERED"
    const val EXTRA_MESSAGE_ID = "message_id"
    const val EXTRA_PART = "part"
    const val EXTRA_PARTS = "parts"

    fun hasSendPermission(context: Context): Boolean =
        ContextCompat.checkSelfPermission(context, Manifest.permission.SEND_SMS) == PackageManager.PERMISSION_GRANTED

    fun prepare(context: Context, body: String, subscriptionId: Int?): Prepared {
        if (!hasSendPermission(context)) {
            return Prepared.Failed(SmsFailure("permission_denied", "The app does not have permission to send SMS on this phone."))
        }
        return try {
            val manager = managerFor(context, subscriptionId)
            val parts = manager.divideMessage(body)
            if (parts.isEmpty()) Prepared.Failed(SmsFailure("invalid_message", "The message is empty.")) else Prepared.Ready(manager, parts)
        } catch (e: Exception) {
            Prepared.Failed(SmsFailure("send_exception", e.message ?: e.javaClass.simpleName))
        }
    }

    fun dispatch(context: Context, prepared: Prepared.Ready, messageId: String, to: String): Outcome {
        val parts = prepared.parts
        val count = parts.size
        return try {
            val sentIntents = ArrayList<PendingIntent>(count)
            val deliveredIntents = ArrayList<PendingIntent?>(count)
            for (i in 0 until count) {
                sentIntents += pending(context, ACTION_SENT, messageId, i, count)
                // Only the last segment carries a delivery request; delivery of
                // the last part is delivery of the message.
                deliveredIntents += if (i == count - 1) pending(context, ACTION_DELIVERED, messageId, i, count) else null
            }
            if (count == 1) {
                prepared.manager.sendTextMessage(to, null, parts[0], sentIntents[0], deliveredIntents[0])
            } else {
                prepared.manager.sendMultipartTextMessage(to, null, parts, sentIntents, ArrayList(deliveredIntents))
            }
            Outcome.Handed(count)
        } catch (e: Exception) {
            Outcome.Failed(SmsFailure("send_exception", e.message ?: e.javaClass.simpleName))
        }
    }

    private fun managerFor(context: Context, subscriptionId: Int?): SmsManager {
        val system = context.getSystemService(SmsManager::class.java)
        return if (subscriptionId != null && subscriptionId >= 0 && SimInfo.isValidSubscription(context, subscriptionId)) {
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) system.createForSubscriptionId(subscriptionId)
            else @Suppress("DEPRECATION") SmsManager.getSmsManagerForSubscriptionId(subscriptionId)
        } else {
            system
        }
    }

    private fun pending(context: Context, action: String, messageId: String, part: Int, parts: Int): PendingIntent {
        val intent = Intent(context, SmsStatusReceiver::class.java)
            .setAction(action)
            .putExtra(EXTRA_MESSAGE_ID, messageId)
            .putExtra(EXTRA_PART, part)
            .putExtra(EXTRA_PARTS, parts)
        // Mutable so the platform can attach its result extras. The request
        // code keeps each message and part distinct.
        val flags = PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_MUTABLE
        return PendingIntent.getBroadcast(context, "$action|$messageId|$part".hashCode(), intent, flags)
    }
}

/** Lets the outbox drainer wait for the sent result of the message it just handed over. */
object SendTracker {
    private val waiters = ConcurrentHashMap<String, CompletableDeferred<Boolean>>()

    fun expect(messageId: String): CompletableDeferred<Boolean> =
        CompletableDeferred<Boolean>().also { waiters[messageId] = it }

    fun complete(messageId: String, ok: Boolean) {
        waiters.remove(messageId)?.complete(ok)
    }

    fun forget(messageId: String) {
        waiters.remove(messageId)
    }
}
