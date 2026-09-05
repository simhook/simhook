package dev.simhook.app

import dev.simhook.app.core.PairLink
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class PairLinkTest {
    @Test
    fun parsesCodeAndServer() {
        val link = PairLink.parse("simhook://pair?code=ABCD-EFGH&api=https%3A%2F%2Fapi.example.com%2F")
        assertEquals("ABCD-EFGH", link?.code)
        assertEquals("https://api.example.com", link?.api)
        assertEquals("api.example.com", link?.host)
    }

    @Test
    fun codeAloneIsEnough() {
        val link = PairLink.parse("  simhook://pair?code=ABCD-EFGH ")
        assertEquals("ABCD-EFGH", link?.code)
        assertNull(link?.api)
    }

    @Test
    fun refusesOtherSchemesAndHosts() {
        assertNull(PairLink.parse("https://pair?code=ABCD-EFGH"))
        assertNull(PairLink.parse("simhook://other?code=ABCD-EFGH"))
        assertNull(PairLink.parse("simhook://pair?api=https://x"))
        assertNull(PairLink.parse(""))
        assertNull(PairLink.parse(null))
        assertNull(PairLink.parse("not a link"))
    }

    @Test
    fun dropsAServerThePhoneMayNotTalkTo() {
        val plain = PairLink.parse("simhook://pair?code=ABCD-EFGH&api=http://api.example.com")
        assertEquals("ABCD-EFGH", plain?.code)
        assertNull(plain?.api)
        assertEquals("http://localhost:8080", PairLink.parse("simhook://pair?code=ABCD-EFGH&api=http://localhost:8080")?.api)
    }

    @Test
    fun allowedApi() {
        assertTrue(PairLink.allowedApi("https://api.simhook.dev"))
        assertTrue(PairLink.allowedApi("http://10.0.2.2:8080"))
        assertTrue(PairLink.allowedApi("http://127.0.0.1:8080"))
        assertFalse(PairLink.allowedApi("http://api.simhook.dev"))
        assertFalse(PairLink.allowedApi("ftp://api.simhook.dev"))
        assertFalse(PairLink.allowedApi("api.simhook.dev"))
        assertFalse(PairLink.allowedApi(""))
    }
}
