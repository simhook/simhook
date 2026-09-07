package dev.simhook.app.ui.onboarding

import androidx.compose.ui.text.AnnotatedString
import androidx.compose.ui.text.input.OffsetMapping
import androidx.compose.ui.text.input.TransformedText
import androidx.compose.ui.text.input.VisualTransformation

/**
 * The eight characters of a pairing code, however they arrive: typed straight
 * through, typed with the dash, pasted with spaces, or carried by a link. The
 * field holds the bare characters and draws the dash the dashboard shows after
 * the fourth one, so the dash is never something to remember or to type.
 */
object PairingCode {
    const val LENGTH = 8
    private const val GROUP = 4

    /** Letters and digits only, upper case, at most eight of them. */
    fun bare(input: String): String = input.filter { it.isLetterOrDigit() }.uppercase().take(LENGTH)

    /** The bare code as the dashboard writes it: a dash after the fourth character. */
    fun display(bare: String): String =
        if (bare.length < GROUP) bare else bare.substring(0, GROUP) + "-" + bare.substring(GROUP)

    fun isComplete(bare: String): Boolean = bare.length == LENGTH

    /** Draws the dash without making it part of the value. */
    val visual: VisualTransformation = VisualTransformation { text ->
        val bare = text.text
        val mapping = object : OffsetMapping {
            // Past the fourth character the drawn text is one longer than the value.
            override fun originalToTransformed(offset: Int): Int =
                if (bare.length >= GROUP && offset >= GROUP) offset + 1 else offset

            // Either side of the dash is the same place in the value.
            override fun transformedToOriginal(offset: Int): Int =
                if (offset > GROUP) offset - 1 else offset
        }
        TransformedText(AnnotatedString(display(bare)), mapping)
    }
}
