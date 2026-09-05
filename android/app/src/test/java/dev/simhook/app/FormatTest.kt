package dev.simhook.app

import dev.simhook.app.ui.formatBytes
import dev.simhook.app.ui.statusLabel
import dev.simhook.app.ui.statusTone
import dev.simhook.app.ui.theme.Tone
import org.junit.Assert.assertEquals
import org.junit.Test

class FormatTest {
    @Test
    fun statusWords() {
        assertEquals("On the phone", statusLabel("dispatched"))
        assertEquals("No result", statusLabel("unknown"))
        assertEquals("Odd", statusLabel("odd"))
    }

    @Test
    fun statusTones() {
        assertEquals(Tone.Ok, statusTone("delivered"))
        assertEquals(Tone.Ok, statusTone("received"))
        assertEquals(Tone.Warn, statusTone("unknown"))
        assertEquals(Tone.Bad, statusTone("failed"))
        assertEquals(Tone.Off, statusTone("queued"))
    }

    @Test
    fun bytes() {
        assertEquals("8.4 MB", formatBytes(8_808_038))
        assertEquals("512 KB", formatBytes(524_288))
        assertEquals("12 B", formatBytes(12))
    }
}
