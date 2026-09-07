package dev.simhook.app.outbox

import android.content.Context
import dev.simhook.app.AppContainer
import dev.simhook.app.core.Notifications
import dev.simhook.app.sms.SendTracker
import dev.simhook.app.sms.SimInfo
import dev.simhook.app.sms.SmsErrors
import dev.simhook.app.sms.SmsFailure
import dev.simhook.app.sms.SmsSender
import dev.simhook.app.work.ReportLedger
import kotlinx.coroutines.delay
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.withTimeoutOrNull

/**
 * Sends what is in the outbox, one message at a time, at the phone's pace
 * and under the platform's segment ceiling. The foreground service runs it
 * without a deadline; the fallback worker runs it with one, when the system
 * refused to start the service. Only one loop runs at a time, whoever asks.
 */
object OutboxDrainer {
    /** How long the radio normally takes to answer; pacing waits this long for it. */
    private const val SENT_TIMEOUT_MS = 45_000L

    /** How long a message may wait on the radio before it is called interrupted. */
    private const val AWAIT_HORIZON_MS = 10 * 60_000L

    /** A claim older than this belongs to a process that died before the radio got anything. */
    private const val CLAIM_HORIZON_MS = 60_000L

    /** How many times a message is handed to the radio before a recoverable failure is final. */
    const val MAX_ATTEMPTS = 6

    private val lock = Mutex()

    /**
     * Returns true when messages are still waiting (the deadline passed, or
     * they are cooling off). With [waitForTurn] the caller waits for a loop
     * that is already running to finish; without it the caller leaves the
     * work to that loop and returns at once.
     */
    suspend fun drain(context: Context, container: AppContainer, deadlineMillis: Long?, waitForTurn: Boolean, onProgress: (remaining: Int) -> Unit): Boolean {
        if (waitForTurn) lock.lock() else if (!lock.tryLock()) return false
        try {
            return run(context, container, deadlineMillis, onProgress)
        } finally {
            lock.unlock()
        }
    }

    private suspend fun run(context: Context, container: AppContainer, deadlineMillis: Long?, onProgress: (remaining: Int) -> Unit): Boolean {
        val dao = container.outbox
        val startedAt = System.currentTimeMillis()

        // Without the permission nothing can be sent, and nothing is touched:
        // the rows wait, the server keeps offering them, and the owner is told.
        if (!SmsSender.hasSendPermission(context)) {
            if (dao.pendingCount() > 0) {
                Notifications.alert(
                    context, Notifications.ID_ALERT_PERMISSION,
                    "SMS permission missing",
                    "Messages are waiting, and nothing is sent until the app may send SMS. Open the app to grant it.",
                )
            }
            return false
        }
        Notifications.cancel(context, Notifications.ID_ALERT_PERMISSION)

        // A message the radio has had for far longer than any answer takes
        // is reported as interrupted: a failure, honestly labelled, that a
        // late result still overturns on both sides.
        for (stuck in dao.interrupted(before = startedAt - AWAIT_HORIZON_MS)) {
            if (dao.finish(stuck.id, OutboxMessage.STATE_INTERRUPTED, startedAt, "No answer from the radio.") > 0) {
                ReportLedger.status(
                    context, container, stuck.id, "failed", startedAt, "interrupted",
                    "The phone handed the message to the radio and never heard back. It may or may not have gone out; a late result will update this.",
                )
            }
        }
        dao.unclaim(before = startedAt - CLAIM_HORIZON_MS, now = startedAt)
        dao.prune(before = startedAt - 7L * 24 * 3600 * 1000)

        val budget = SegmentBudget.forPhone(context)
        budget.seed(dao.segmentsSince(startedAt - budget.periodMs), startedAt)

        while (true) {
            val settings = container.settings.current()
            if (!settings.isPaired || !settings.gatewayEnabled) return false
            var now = System.currentTimeMillis()
            val next = dao.nextPending(now)
            if (next == null) {
                // Nothing may go right now. Wait for the earliest cool-off to
                // pass, or hand the wait to the caller when there is a deadline.
                val wake = dao.earliestNotBefore(now) ?: return false
                if (deadlineMillis != null && wake > deadlineMillis) return true
                delay((wake - now).coerceIn(1_000L, 60_000L))
                continue
            }
            if (deadlineMillis != null && now > deadlineMillis) return true
            onProgress(dao.inFlightCount())

            // A SIM the request named that this phone can see is not in it
            // any more falls back to the preferred one, then to the phone's
            // default. Without the permission to see SIMs the id is trusted:
            // a wrong one fails visibly rather than sending from the wrong SIM.
            val canSeeSims = SimInfo.hasPermission(context)
            val requested = next.simSubscriptionId?.takeIf { !canSeeSims || SimInfo.isValidSubscription(context, it) }
            val sim = requested ?: settings.preferredSimSubscriptionId?.takeIf { !canSeeSims || SimInfo.isValidSubscription(context, it) }
            when (val prepared = SmsSender.prepare(context, next.body, sim)) {
                is SmsSender.Prepared.Failed -> {
                    if (dao.claim(next.id, now) == 0) continue
                    dao.markHanded(next.id, 1, now)
                    settle(context, container, next.id, now, prepared.failure)
                }
                is SmsSender.Prepared.Ready -> {
                    val parts = prepared.parts.size
                    val wait = budget.waitFor(parts, now)
                    if (wait > 0) {
                        if (deadlineMillis != null && now + wait > deadlineMillis) return true
                        delay(wait)
                        now = System.currentTimeMillis()
                    }
                    if (dao.claim(next.id, now) == 0) continue
                    // The row knows how many parts to expect before the radio
                    // can report on any of them.
                    dao.markHanded(next.id, parts, now)
                    budget.record(parts, now)
                    val waiter = SendTracker.expect(next.id)
                    when (val outcome = SmsSender.dispatch(context, prepared, next.id, next.to)) {
                        is SmsSender.Outcome.Failed -> {
                            SendTracker.forget(next.id)
                            settle(context, container, next.id, now, outcome.failure)
                        }
                        is SmsSender.Outcome.Handed -> {
                            // Wait for the radio's verdict so pacing counts real
                            // sends. A radio that says nothing keeps the row, off
                            // this loop's path, until it does or the horizon passes.
                            val verdict = withTimeoutOrNull(SENT_TIMEOUT_MS) { waiter.await() }
                            SendTracker.forget(next.id)
                            if (verdict == null) dao.await(next.id, System.currentTimeMillis())
                        }
                    }
                }
            }
            delay(settings.sendDelaySeconds.coerceIn(0, 3600) * 1000L)
        }
    }

    /**
     * What becomes of a message the radio did not take. A condition the
     * phone can recover from (no service, the platform's own limit) puts the
     * message back in the queue with a cool-off, and only a spent retry
     * budget makes it a failure; everything else is one. Returns true when
     * the message ended for good.
     */
    suspend fun settle(context: Context, container: AppContainer, id: String, now: Long, failure: SmsFailure): Boolean {
        val dao = container.outbox
        if (SmsErrors.isTransient(failure.code)) {
            val attempts = dao.get(id)?.attempts ?: MAX_ATTEMPTS
            if (attempts < MAX_ATTEMPTS && dao.retryLater(id, now + coolOff(attempts), now, failure.message) > 0) return false
        }
        if (dao.finish(id, OutboxMessage.STATE_FAILED, now, failure.message) > 0) {
            ReportLedger.status(context, container, id, "failed", now, failure.code, failure.message)
        }
        return true
    }

    /** How long to leave a message alone after its [attempts]th try met a passing condition. */
    fun coolOff(attempts: Int): Long {
        val base = 60_000L shl (attempts - 1).coerceIn(0, 4)
        return base.coerceAtMost(15 * 60_000L)
    }
}
