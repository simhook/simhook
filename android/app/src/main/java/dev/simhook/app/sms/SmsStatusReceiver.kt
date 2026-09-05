package dev.simhook.app.sms

import android.app.Activity
import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.telephony.SmsMessage
import dev.simhook.app.SimhookApp
import dev.simhook.app.outbox.OutboxMessage
import dev.simhook.app.work.StatusReportWorker
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch

/**
 * Receives the sent and delivered results the radio reports for messages
 * handed over by [SmsSender], updates the outbox, and queues the report to
 * the server. Each outbox step is one conditional update, so the parts of
 * a long text arriving together are counted once each and the message is
 * finished exactly once.
 */
class SmsStatusReceiver : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        val messageId = intent.getStringExtra(SmsSender.EXTRA_MESSAGE_ID) ?: return
        val code = resultCode
        val radioError = intent.getIntExtra("errorCode", -1).takeIf { it != -1 }
        val action = intent.action ?: return
        // A delivery report carries the carrier's own verdict in a status
        // report PDU; the result code alone says little on many phones.
        val pduStatus = if (action == SmsSender.ACTION_DELIVERED) statusReportOf(intent) else null
        val pending = goAsync()
        CoroutineScope(Dispatchers.IO).launch {
            try {
                handle(context.applicationContext, action, messageId, code, radioError, pduStatus)
            } finally {
                pending.finish()
            }
        }
    }

    private fun statusReportOf(intent: Intent): Int? {
        val pdu = intent.getByteArrayExtra("pdu") ?: return null
        val format = intent.getStringExtra("format") ?: "3gpp"
        return runCatching { SmsMessage.createFromPdu(pdu, format)?.status }.getOrNull()
    }

    private suspend fun handle(context: Context, action: String, messageId: String, code: Int, radioError: Int?, pduStatus: Int?) {
        val dao = SimhookApp.get(context).container.outbox
        val now = System.currentTimeMillis()
        when (action) {
            SmsSender.ACTION_SENT -> {
                if (code == Activity.RESULT_OK) {
                    if (dao.partOk(messageId, now) > 0 && dao.completeIfAllParts(messageId, now) > 0) {
                        StatusReportWorker.enqueue(context, messageId, "sent", now, null, null)
                        SendTracker.complete(messageId, true)
                    }
                } else {
                    val failure = SmsErrors.forSentResult(code, radioError)
                    if (dao.finish(messageId, OutboxMessage.STATE_FAILED, now, failure.message) > 0) {
                        StatusReportWorker.enqueue(context, messageId, "failed", now, failure.code, failure.message)
                        SendTracker.complete(messageId, false)
                    }
                }
            }
            SmsSender.ACTION_DELIVERED -> when (SmsErrors.classifyDelivery(code, pduStatus)) {
                SmsErrors.DeliveryOutcome.Delivered -> StatusReportWorker.enqueue(context, messageId, "delivered", now, null, null)
                SmsErrors.DeliveryOutcome.Pending -> Unit
                SmsErrors.DeliveryOutcome.Failed -> {
                    val failure = SmsErrors.deliveryFailure(code, pduStatus)
                    dao.setState(messageId, OutboxMessage.STATE_FAILED, now, failure.message)
                    StatusReportWorker.enqueue(context, messageId, "failed", now, failure.code, failure.message)
                }
            }
        }
    }
}
