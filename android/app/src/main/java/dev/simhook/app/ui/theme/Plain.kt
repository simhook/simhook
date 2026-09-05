package dev.simhook.app.ui.theme

import androidx.compose.foundation.background
import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.ColumnScope
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.layout.width
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.material3.Button
import androidx.compose.material3.ButtonDefaults
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.OutlinedTextFieldDefaults
import androidx.compose.material3.Slider
import androidx.compose.material3.SliderDefaults
import androidx.compose.material3.Switch
import androidx.compose.material3.SwitchDefaults
import androidx.compose.material3.Text
import androidx.compose.material3.TextFieldColors
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.graphics.RectangleShape
import androidx.compose.ui.text.style.TextDecoration
import androidx.compose.ui.unit.dp

/**
 * The few pieces every screen is made of: a rule, a section under a mono
 * label, a row with a rule under it, a dot next to a word, a text link, and
 * the one filled button a screen may have.
 */

enum class Tone { Ok, Warn, Bad, Off }

private fun Tone.color(): Color = when (this) {
    Tone.Ok -> Tokens.Ok
    Tone.Warn -> Tokens.Warn
    Tone.Bad -> Tokens.Bad
    Tone.Off -> Tokens.DotOff
}

@Composable
fun Hairline(modifier: Modifier = Modifier) {
    HorizontalDivider(modifier = modifier, thickness = 1.dp, color = Tokens.Line)
}

@Composable
fun SectionLabel(text: String, modifier: Modifier = Modifier) {
    Text(text, style = MaterialTheme.typography.labelSmall, color = Tokens.Muted, modifier = modifier)
}

/** A mono label, a rule, and whatever the section holds. */
@Composable
fun Section(label: String, modifier: Modifier = Modifier, content: @Composable ColumnScope.() -> Unit) {
    Column(modifier.fillMaxWidth()) {
        SectionLabel(label)
        Spacer(Modifier.height(6.dp))
        Hairline()
        content()
    }
}

/** A row of a list: a title, an optional line under it, something at the end, and a rule below. */
@Composable
fun ListRow(
    title: String,
    subtitle: String? = null,
    onClick: (() -> Unit)? = null,
    trailing: (@Composable () -> Unit)? = null,
) {
    Column(Modifier.fillMaxWidth()) {
        Row(
            modifier = Modifier
                .fillMaxWidth()
                .then(if (onClick != null) Modifier.clickable(onClick = onClick) else Modifier)
                .padding(vertical = 10.dp),
            verticalAlignment = Alignment.CenterVertically,
            horizontalArrangement = Arrangement.spacedBy(16.dp),
        ) {
            Column(Modifier.weight(1f)) {
                Text(title, style = MaterialTheme.typography.bodyMedium)
                if (subtitle != null) Text(subtitle, style = MaterialTheme.typography.bodySmall, color = Tokens.Muted)
            }
            trailing?.invoke()
        }
        Hairline()
    }
}

/** A status is a dot next to a word. */
@Composable
fun StatusWord(tone: Tone, word: String, modifier: Modifier = Modifier, muted: Boolean = false) {
    Row(modifier, verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(8.dp)) {
        Box(
            Modifier
                .size(7.dp)
                .background(tone.color(), CircleShape),
        )
        Text(word, style = MaterialTheme.typography.bodySmall, color = if (muted) Tokens.Muted else Tokens.Fg)
    }
}

/** An action that is not the screen's button: a word with a line under it. */
@Composable
fun TextLink(text: String, onClick: () -> Unit, modifier: Modifier = Modifier, enabled: Boolean = true, tone: Tone? = null) {
    val color = when {
        !enabled -> Tokens.Muted
        tone == Tone.Bad -> Tokens.Bad
        else -> Tokens.Fg
    }
    Text(
        text,
        style = MaterialTheme.typography.bodyMedium.copy(textDecoration = TextDecoration.Underline),
        color = color,
        modifier = modifier
            .clickable(enabled = enabled, onClick = onClick)
            .padding(vertical = 4.dp),
    )
}

/** The one filled button a screen may have. */
@Composable
fun FilledButton(text: String, onClick: () -> Unit, modifier: Modifier = Modifier, enabled: Boolean = true) {
    Button(
        onClick = onClick,
        enabled = enabled,
        modifier = modifier,
        shape = RectangleShape,
        colors = ButtonDefaults.buttonColors(
            containerColor = Tokens.Fg,
            contentColor = Tokens.Bg,
            disabledContainerColor = Tokens.Fg.copy(alpha = 0.4f),
            disabledContentColor = Tokens.Bg,
        ),
    ) {
        Text(text, style = MaterialTheme.typography.labelLarge)
    }
}

/** A notice is a paragraph with a rule on its left, not a box. */
@Composable
fun Note(title: String, text: String? = null, tone: Tone = Tone.Bad, modifier: Modifier = Modifier, action: (@Composable () -> Unit)? = null) {
    Row(modifier.fillMaxWidth()) {
        Box(
            Modifier
                .width(2.dp)
                .height(if (text == null) 24.dp else 48.dp)
                .background(if (tone == Tone.Off) Tokens.Fg else tone.color()),
        )
        Spacer(Modifier.width(14.dp))
        Column {
            Text(title, style = MaterialTheme.typography.titleMedium, color = if (tone == Tone.Bad) Tokens.Bad else Tokens.Fg)
            if (text != null) Text(text, style = MaterialTheme.typography.bodySmall, color = Tokens.Muted)
            action?.invoke()
        }
    }
}

/** A number with a mono label under it. */
@Composable
fun Stat(value: String, label: String, modifier: Modifier = Modifier) {
    Column(modifier) {
        Text(value, style = MaterialTheme.typography.headlineSmall)
        Text(label, style = MaterialTheme.typography.labelSmall, color = Tokens.Muted)
    }
}

@Composable
fun PlainSwitch(checked: Boolean, onCheckedChange: (Boolean) -> Unit, enabled: Boolean = true) {
    Switch(
        checked = checked,
        onCheckedChange = onCheckedChange,
        enabled = enabled,
        colors = SwitchDefaults.colors(
            checkedThumbColor = Tokens.Bg,
            checkedTrackColor = Tokens.Fg,
            checkedBorderColor = Color.Transparent,
            uncheckedThumbColor = Tokens.Bg,
            uncheckedTrackColor = Tokens.DotOff,
            uncheckedBorderColor = Color.Transparent,
            disabledCheckedThumbColor = Tokens.Bg,
            disabledCheckedTrackColor = Tokens.Fg.copy(alpha = 0.4f),
            disabledUncheckedThumbColor = Tokens.Bg,
            disabledUncheckedTrackColor = Tokens.Line,
        ),
    )
}

@Composable
fun PlainSlider(value: Float, onValueChange: (Float) -> Unit, onValueChangeFinished: () -> Unit, valueRange: ClosedFloatingPointRange<Float>, steps: Int, enabled: Boolean) {
    Slider(
        value = value,
        onValueChange = onValueChange,
        onValueChangeFinished = onValueChangeFinished,
        valueRange = valueRange,
        steps = steps,
        enabled = enabled,
        colors = SliderDefaults.colors(
            thumbColor = Tokens.Fg,
            activeTrackColor = Tokens.Fg,
            inactiveTrackColor = Tokens.Line,
            activeTickColor = Color.Transparent,
            inactiveTickColor = Color.Transparent,
        ),
    )
}

@Composable
fun plainFieldColors(): TextFieldColors = OutlinedTextFieldDefaults.colors(
    focusedBorderColor = Tokens.Fg,
    unfocusedBorderColor = Tokens.Line,
    cursorColor = Tokens.Fg,
    focusedLabelColor = Tokens.Muted,
    unfocusedLabelColor = Tokens.Muted,
    focusedTextColor = Tokens.Fg,
    unfocusedTextColor = Tokens.Fg,
    focusedPlaceholderColor = Tokens.DotOff,
    unfocusedPlaceholderColor = Tokens.DotOff,
)

@Composable
fun PlainTextField(
    value: String,
    onValueChange: (String) -> Unit,
    label: String,
    placeholder: String? = null,
    modifier: Modifier = Modifier,
    singleLine: Boolean = true,
    keyboardOptions: androidx.compose.foundation.text.KeyboardOptions = androidx.compose.foundation.text.KeyboardOptions.Default,
) {
    OutlinedTextField(
        value = value,
        onValueChange = onValueChange,
        label = { Text(label) },
        placeholder = placeholder?.let { { Text(it) } },
        singleLine = singleLine,
        keyboardOptions = keyboardOptions,
        shape = RectangleShape,
        colors = plainFieldColors(),
        modifier = modifier,
    )
}
