package dev.simhook.app

import dev.simhook.app.sms.SmsReceivedReceiver
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class FingerprintTest {
    @Test
    fun stableForTheSameText() {
        val a = SmsReceivedReceiver.fingerprint("+15550001111", "hello", 1_700_000_000_000L)
        val b = SmsReceivedReceiver.fingerprint("+15550001111", "hello", 1_700_000_000_000L)
        assertEquals(a, b)
        assertEquals(64, a.length)
        assertTrue(a.all { it in '0'..'9' || it in 'a'..'f' })
    }

    @Test
    fun differentForDifferentTexts() {
        val base = SmsReceivedReceiver.fingerprint("+15550001111", "hello", 1L)
        assertNotEquals(base, SmsReceivedReceiver.fingerprint("+15550001111", "hello!", 1L))
        assertNotEquals(base, SmsReceivedReceiver.fingerprint("+15550001112", "hello", 1L))
        assertNotEquals(base, SmsReceivedReceiver.fingerprint("+15550001111", "hello", 2L))
    }

    @Test
    fun longTextsAreCutWithAMark() {
        val long = "x".repeat(SmsReceivedReceiver.MAX_BODY + 50)
        val cut = SmsReceivedReceiver.truncate(long)
        assertEquals(SmsReceivedReceiver.MAX_BODY, cut.length)
        assertTrue(cut.endsWith("\u2026"))
        assertEquals("short", SmsReceivedReceiver.truncate("short"))
    }
}
