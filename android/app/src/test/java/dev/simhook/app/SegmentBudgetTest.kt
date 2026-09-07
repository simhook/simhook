package dev.simhook.app

import dev.simhook.app.outbox.OutboxDrainer
import dev.simhook.app.outbox.SegmentBudget
import dev.simhook.app.sms.SmsErrors
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class SegmentBudgetTest {
    private val period = 60_000L
    private val margin = SegmentBudget.MARGIN_MS

    @Test
    fun `fits until the window is full`() {
        val b = SegmentBudget(maxCount = 30, periodMs = period)
        var now = 1_000_000L
        repeat(7) {
            assertEquals(0L, b.waitFor(4, now))
            b.record(4, now)
            now += 5_000
        }
        // 28 segments in the window; two more fit, three do not.
        assertEquals(0L, b.waitFor(2, now))
        assertTrue(b.waitFor(3, now) > 0)
    }

    @Test
    fun `waits for exactly enough old segments to leave the window`() {
        val b = SegmentBudget(maxCount = 30, periodMs = period)
        val t0 = 1_000_000L
        b.record(10, t0)
        b.record(10, t0 + 10_000)
        b.record(10, t0 + 20_000)
        val now = t0 + 30_000
        // Four more need four of the oldest ten gone: the first batch expires at t0 + period.
        assertEquals(t0 + period + margin - now, b.waitFor(4, now))
        // Fourteen need all of the first batch and four of the second.
        assertEquals(t0 + 10_000 + period + margin - now, b.waitFor(14, now))
    }

    @Test
    fun `old segments fall out of the window`() {
        val b = SegmentBudget(maxCount = 30, periodMs = period)
        b.record(30, 1_000_000L)
        assertTrue(b.waitFor(1, 1_000_000L + 30_000) > 0)
        assertEquals(0L, b.waitFor(30, 1_000_000L + period + 1))
    }

    @Test
    fun `a message longer than the whole window waits for an empty one and then goes`() {
        val b = SegmentBudget(maxCount = 30, periodMs = period)
        assertEquals(0L, b.waitFor(31, 1_000_000L))
        b.record(1, 1_000_000L)
        assertEquals(period + margin - 5_000, b.waitFor(31, 1_000_000L + 5_000))
    }

    @Test
    fun `seeded segments count as just sent`() {
        val b = SegmentBudget(maxCount = 30, periodMs = period)
        b.seed(28, 1_000_000L)
        assertEquals(0L, b.waitFor(2, 1_000_000L))
        assertTrue(b.waitFor(3, 1_000_000L) > 0)
    }

    @Test
    fun `cool-off doubles and is capped`() {
        assertEquals(60_000L, OutboxDrainer.coolOff(1))
        assertEquals(120_000L, OutboxDrainer.coolOff(2))
        assertEquals(240_000L, OutboxDrainer.coolOff(3))
        assertEquals(480_000L, OutboxDrainer.coolOff(4))
        assertEquals(15 * 60_000L, OutboxDrainer.coolOff(5))
        assertEquals(15 * 60_000L, OutboxDrainer.coolOff(9))
        assertEquals(60_000L, OutboxDrainer.coolOff(0))
    }

    @Test
    fun `only passing conditions are retried`() {
        assertTrue(SmsErrors.isTransient("rate_limited"))
        assertTrue(SmsErrors.isTransient("no_service"))
        assertTrue(SmsErrors.isTransient("radio_off"))
        assertFalse(SmsErrors.isTransient("generic_failure"))
        assertFalse(SmsErrors.isTransient("invalid_message"))
        assertFalse(SmsErrors.isTransient("permission_denied"))
        assertFalse(SmsErrors.isTransient("fdn_blocked"))
    }
}
