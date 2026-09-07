package dev.simhook.app.outbox

import android.content.Context
import android.provider.Settings

/**
 * Keeps this app under the platform's ceiling for outgoing texts. Android
 * meters an app that is not the phone's messenger by segment: 30 in any 60
 * seconds by default, read here from the phone's own settings in case a
 * build changes them. Over the ceiling the platform does not fail the send;
 * it puts a dialog on the screen and waits for a human, and after a few of
 * those refuses everything until reboot. So the budget is honoured before
 * the radio is asked, by waiting until enough earlier segments have left
 * the window.
 */
class SegmentBudget(val maxCount: Int, val periodMs: Long) {
    private val sent = ArrayDeque<Long>()

    /** Segments handed to the radio before this budget existed, counted as if just sent. */
    fun seed(segments: Int, now: Long) {
        repeat(segments.coerceAtLeast(0)) { sent.addLast(now) }
    }

    /** How long to wait before [parts] more segments fit in the window; zero when they fit now. */
    fun waitFor(parts: Int, now: Long): Long {
        while (sent.isNotEmpty() && sent.first() <= now - periodMs) sent.removeFirst()
        if (parts >= maxCount) return if (sent.isEmpty()) 0L else sent.last() + periodMs + MARGIN_MS - now
        val over = sent.size + parts - maxCount
        if (over <= 0) return 0L
        // The oldest `over` entries must leave the window first.
        return (sent[over - 1] + periodMs + MARGIN_MS - now).coerceAtLeast(0L)
    }

    /** [parts] segments were just handed to the radio. */
    fun record(parts: Int, now: Long) {
        repeat(parts) { sent.addLast(now) }
    }

    companion object {
        /** Slack over the platform's own clock, which started counting before ours did. */
        const val MARGIN_MS = 1_500L
        const val DEFAULT_MAX_COUNT = 30
        const val DEFAULT_PERIOD_MS = 60_000L

        /** The limits this phone enforces, as the platform reads them. */
        fun forPhone(context: Context): SegmentBudget {
            val resolver = context.contentResolver
            val max = runCatching { Settings.Global.getInt(resolver, "sms_outgoing_check_max_count", DEFAULT_MAX_COUNT) }.getOrDefault(DEFAULT_MAX_COUNT)
            val period = runCatching { Settings.Global.getLong(resolver, "sms_outgoing_check_interval_ms", DEFAULT_PERIOD_MS) }.getOrDefault(DEFAULT_PERIOD_MS)
            return SegmentBudget(max.coerceAtLeast(1), period.coerceAtLeast(1_000L))
        }
    }
}
