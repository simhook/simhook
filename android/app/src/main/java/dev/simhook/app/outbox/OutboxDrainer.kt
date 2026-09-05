package dev.simhook.app.outbox

import android.content.Context
import dev.simhook.app.AppContainer
import dev.simhook.app.sms.SendTracker
import dev.simhook.app.sms.SimInfo
import dev.simhook.app.sms.SmsFailure
import dev.simhook.app.sms.SmsSender
import dev.simhook.app.work.StatusReportWorker
import kotlinx.coroutines.delay
import kotlinx.coroutines.withTimeoutOrNull

/**
 * Sends what is in the outbox, one message at a time, at the phone's pace.
 * The foreground service runs it without a deadline; the fallback worker
 * runs it with one, when the system refused to start the service.
 */
object OutboxDrainer {
    private const val SENT_TIMEOUT_MS = 45_000L

    /** Returns true when messages are still waiting (the deadline passed). */
    suspend fun drain(context: Context, container: AppContainer, deadlineMillis: Long?, onProgress: (remaining: Int) -> Unit): Boolean {
        val dao = container.outbox
        val startedAt = System.currentTimeMillis()

        // Anything left mid-send by a previous process life is reported as
        // failed with an honest reason, since we cannot know whether the
        // radio sent it.
        for (stuck in dao.interrupted(before = startedAt - 60_000)) {
            if (dao.finish(stuck.id, OutboxMessage.STATE_FAILED, startedAt, "Interrupted while sending.") > 0) {
                StatusReportWorker.enqueue(context, stuck.id, "failed", startedAt, "interrupted", "The app was interrupted while sending. The message may or may not have gone out.")
            }
        }
        dao.prune(before = startedAt - 7L * 24 * 3600 * 1000)

        while (true) {
            val settings = container.settings.current()
            if (!settings.isPaired || !settings.gatewayEnabled) return false
            val next = dao.nextPending() ?: return false
            if (deadlineMillis != null && System.currentTimeMillis() > deadlineMillis) return true
            onProgress(dao.inFlightCount())

            // A SIM the request named that is not in the phone any more falls
            // back to the preferred one, then to the phone's default.
            val requested = next.simSubscriptionId?.takeIf { SimInfo.isValidSubscription(context, it) }
            val sim = requested ?: settings.preferredSimSubscriptionId
            val t = System.currentTimeMillis()
            when (val prepared = SmsSender.prepare(context, next.body, sim)) {
                is SmsSender.Prepared.Failed -> {
                    dao.markHanded(next.id, 1, t)
                    fail(context, dao, next.id, t, prepared.failure)
                }
                is SmsSender.Prepared.Ready -> {
                    // The row knows how many parts to expect before the radio
                    // can report on any of them.
                    dao.markHanded(next.id, prepared.parts.size, t)
                    val waiter = SendTracker.expect(next.id)
                    when (val outcome = SmsSender.dispatch(context, prepared, next.id, next.to)) {
                        is SmsSender.Outcome.Failed -> {
                            SendTracker.forget(next.id)
                            fail(context, dao, next.id, t, outcome.failure)
                        }
                        is SmsSender.Outcome.Handed -> {
                            // Wait for the radio's verdict so pacing counts real sends.
                            withTimeoutOrNull(SENT_TIMEOUT_MS) { waiter.await() }
                            SendTracker.forget(next.id)
                        }
                    }
                }
            }
            delay(settings.sendDelaySeconds.coerceIn(0, 3600) * 1000L)
        }
    }

    private suspend fun fail(context: Context, dao: OutboxDao, id: String, at: Long, failure: SmsFailure) {
        if (dao.finish(id, OutboxMessage.STATE_FAILED, at, failure.message) > 0) {
            StatusReportWorker.enqueue(context, id, "failed", at, failure.code, failure.message)
        }
    }
}
