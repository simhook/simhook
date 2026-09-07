package dev.simhook.app.sms

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.provider.Telephony
import android.util.Log
import dev.simhook.app.SimhookApp
import dev.simhook.app.work.ReportLedger
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import java.security.MessageDigest

/**
 * Observes incoming SMS. The app is never the default messaging app, so it
 * only sees the broadcast; the message still lands in the inbox as usual.
 * The text is written down before anything else happens to it, because the
 * app has no other copy: it is not the messenger and cannot read the inbox.
 */
class SmsReceivedReceiver : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        if (intent.action != Telephony.Sms.Intents.SMS_RECEIVED_ACTION) return
        val messages = Telephony.Sms.Intents.getMessagesFromIntent(intent)?.takeIf { it.isNotEmpty() } ?: return
        val sender = messages[0].originatingAddress ?: return
        val body = truncate(messages.joinToString("") { it.messageBody ?: "" })
        if (body.isEmpty()) return
        val receivedAt = messages[0].timestampMillis
        val subscriptionId = intent.getIntExtra("subscription", -1).takeIf { it >= 0 }
        val fingerprint = fingerprint(sender, body, receivedAt)

        val pending = goAsync()
        val app = context.applicationContext
        CoroutineScope(Dispatchers.IO).launch {
            try {
                val container = SimhookApp.get(app).container
                val settings = container.settings.current()
                if (!settings.isPaired || !settings.receiveEnabled) return@launch
                ReportLedger.inbound(app, container, sender, body, receivedAt, fingerprint, subscriptionId)
            } catch (e: Exception) {
                // A text that cannot be recorded must not take the process down with it.
                Log.w(TAG, "recording a received text failed", e)
            } finally {
                pending.finish()
            }
        }
    }

    companion object {
        private const val TAG = "SmsReceived"

        /** The most the API stores for one text. Longer ones are cut with a mark, not dropped. */
        const val MAX_BODY = 20_000

        fun truncate(body: String): String = if (body.length <= MAX_BODY) body else body.take(MAX_BODY - 1) + "…"

        /** Stable identity for a received message, so retries and re-broadcasts never duplicate it. */
        fun fingerprint(sender: String, body: String, receivedAt: Long): String {
            val digest = MessageDigest.getInstance("SHA-256").digest("$sender $body $receivedAt".toByteArray(Charsets.UTF_8))
            return digest.joinToString("") { "%02x".format(it) }.take(64)
        }
    }
}
