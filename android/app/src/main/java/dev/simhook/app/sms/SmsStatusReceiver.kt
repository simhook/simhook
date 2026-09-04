package dev.simhook.app.sms

import android.app.Activity
import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import dev.simhook.app.SimhookApp
import dev.simhook.app.outbox.OutboxMessage
import dev.simhook.app.work.StatusReportWorker
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch

/**
 * Receives the sent and delivered results the radio reports for messages
 * handed over by [SmsSender], updates the outbox, and queues the report to
 * the server.
 */
class SmsStatusReceiver : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        val messageId = intent.getStringExtra(SmsSender.EXTRA_MESSAGE_ID) ?: return
        val parts = intent.getIntExtra(SmsSender.EXTRA_PARTS, 1)
        val code = resultCode
        val radioError = intent.getIntExtra("errorCode", -1).takeIf { it != -1 }
        val action = intent.action ?: return
        val pending = goAsync()
        CoroutineScope(Dispatchers.IO).launch {
            try {
                handle(context.applicationContext, action, messageId, parts, code, radioError)
            } finally {
                pending.finish()
            }
        }
    }

    private suspend fun handle(context: Context, action: String, messageId: String, parts: Int, code: Int, radioError: Int?) {
        val dao = SimhookApp.get(context).container.outbox
        val row = dao.get(messageId)
        val now = System.currentTimeMillis()
        when (action) {
            SmsSender.ACTION_SENT -> {
                if (row == null) return
                if (row.state == OutboxMessage.STATE_SENT || row.state == OutboxMessage.STATE_FAILED) return
                if (code == Activity.RESULT_OK) {
                    dao.partOk(messageId, now)
                    val ok = (row.partsOk + 1) >= parts
                    if (ok) {
                        dao.setState(messageId, OutboxMessage.STATE_SENT, now, null)
                        StatusReportWorker.enqueue(context, messageId, "sent", now, null, null)
                        SendTracker.complete(messageId, true)
                    }
                } else {
                    val failure = SmsErrors.forSentResult(code, radioError)
                    dao.setState(messageId, OutboxMessage.STATE_FAILED, now, failure.message)
                    StatusReportWorker.enqueue(context, messageId, "failed", now, failure.code, failure.message)
                    SendTracker.complete(messageId, false)
                }
            }
            SmsSender.ACTION_DELIVERED -> {
                val failure = SmsErrors.forDeliveryResult(code)
                if (failure == null) {
                    if (code == Activity.RESULT_OK) {
                        StatusReportWorker.enqueue(context, messageId, "delivered", now, null, null)
                    }
                } else {
                    dao.setState(messageId, OutboxMessage.STATE_FAILED, now, failure.message)
                    StatusReportWorker.enqueue(context, messageId, "failed", now, failure.code, failure.message)
                }
            }
        }
    }
}
