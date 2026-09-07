package dev.simhook.app

import androidx.compose.ui.text.AnnotatedString
import dev.simhook.app.ui.onboarding.PairingCode
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class PairingCodeTest {
    @Test
    fun `whatever shape the code arrives in, the bare form is the eight characters`() {
        assertEquals("6KVK7CNH", PairingCode.bare("6KVK-7CNH"))
        assertEquals("6KVK7CNH", PairingCode.bare("6kvk7cnh"))
        assertEquals("6KVK7CNH", PairingCode.bare(" 6kvk 7cnh "))
        assertEquals("6KVK7CNH", PairingCode.bare("6KVK-7CNH-XY"))
        assertEquals("", PairingCode.bare("--"))
    }

    @Test
    fun `the dash appears once four characters are in and never earlier`() {
        assertEquals("", PairingCode.display(""))
        assertEquals("6KV", PairingCode.display("6KV"))
        assertEquals("6KVK-", PairingCode.display("6KVK"))
        assertEquals("6KVK-7", PairingCode.display("6KVK7"))
        assertEquals("6KVK-7CNH", PairingCode.display("6KVK7CNH"))
    }

    @Test
    fun `only eight characters make a complete code`() {
        assertFalse(PairingCode.isComplete("6KVK7CN"))
        assertTrue(PairingCode.isComplete("6KVK7CNH"))
    }

    @Test
    fun `the drawn text carries the dash and the cursor maps across it`() {
        val drawn = PairingCode.visual.filter(AnnotatedString("6KVK7"))
        assertEquals("6KVK-7", drawn.text.text)
        val m = drawn.offsetMapping
        // Before the group boundary nothing shifts.
        assertEquals(3, m.originalToTransformed(3))
        assertEquals(3, m.transformedToOriginal(3))
        // The end of the first group sits after the drawn dash.
        assertEquals(5, m.originalToTransformed(4))
        assertEquals(6, m.originalToTransformed(5))
        // Either side of the dash is the same place in the value.
        assertEquals(4, m.transformedToOriginal(4))
        assertEquals(4, m.transformedToOriginal(5))
        assertEquals(5, m.transformedToOriginal(6))
    }

    @Test
    fun `a first group with nothing after it still draws the dash and maps its end`() {
        val drawn = PairingCode.visual.filter(AnnotatedString("6KVK"))
        assertEquals("6KVK-", drawn.text.text)
        assertEquals(5, drawn.offsetMapping.originalToTransformed(4))
        assertEquals(4, drawn.offsetMapping.transformedToOriginal(5))
        val short = PairingCode.visual.filter(AnnotatedString("6KV"))
        assertEquals("6KV", short.text.text)
        assertEquals(3, short.offsetMapping.originalToTransformed(3))
    }
}
