package dev.simhook.app.ui.theme

import androidx.compose.foundation.shape.RoundedCornerShape
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Shapes
import androidx.compose.material3.Typography
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.text.ExperimentalTextApi
import androidx.compose.ui.text.TextStyle
import androidx.compose.ui.text.font.Font
import androidx.compose.ui.text.font.FontFamily
import androidx.compose.ui.text.font.FontStyle
import androidx.compose.ui.text.font.FontVariation
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.compose.ui.unit.sp
import dev.simhook.app.R

/**
 * The plain design (docs/decisions.md 014 and 017), on the phone: the five
 * colours of the site and the dashboard, a sans for words, a mono for
 * anything a machine wrote, square corners, and no dark theme.
 */
object Tokens {
    val Fg = Color(0xFF111111)
    val Bg = Color(0xFFFFFFFF)
    val Muted = Color(0xFF6B6B68)
    val Line = Color(0xFFE6E6E3)
    val Soft = Color(0xFFF6F6F4)
    val Ok = Color(0xFF15803D)
    val Warn = Color(0xFFB45309)
    val Bad = Color(0xFFB91C1C)
    val DotOff = Color(0xFFC9C9C5)
    val Underline = Color(0xFFB8B8B4)
}

@OptIn(ExperimentalTextApi::class)
private fun variable(res: Int, weight: FontWeight) =
    Font(res, weight, FontStyle.Normal, variationSettings = FontVariation.Settings(weight, FontStyle.Normal))

val Sans = FontFamily(
    variable(R.font.instrument_sans_variable, FontWeight.Normal),
    variable(R.font.instrument_sans_variable, FontWeight.Medium),
    variable(R.font.instrument_sans_variable, FontWeight.SemiBold),
)

val Mono = FontFamily(
    variable(R.font.geist_mono_variable, FontWeight.Normal),
    variable(R.font.geist_mono_variable, FontWeight.Medium),
)

private val LightColors = lightColorScheme(
    primary = Tokens.Fg,
    onPrimary = Tokens.Bg,
    primaryContainer = Tokens.Soft,
    onPrimaryContainer = Tokens.Fg,
    secondary = Tokens.Muted,
    onSecondary = Tokens.Bg,
    secondaryContainer = Tokens.Soft,
    onSecondaryContainer = Tokens.Fg,
    tertiary = Tokens.Muted,
    onTertiary = Tokens.Bg,
    background = Tokens.Bg,
    onBackground = Tokens.Fg,
    surface = Tokens.Bg,
    onSurface = Tokens.Fg,
    surfaceVariant = Tokens.Soft,
    onSurfaceVariant = Tokens.Muted,
    surfaceContainerLowest = Tokens.Bg,
    surfaceContainerLow = Tokens.Bg,
    surfaceContainer = Tokens.Bg,
    surfaceContainerHigh = Tokens.Bg,
    surfaceContainerHighest = Tokens.Soft,
    inverseSurface = Tokens.Fg,
    inverseOnSurface = Tokens.Bg,
    inversePrimary = Tokens.Bg,
    outline = Tokens.Line,
    outlineVariant = Tokens.Line,
    error = Tokens.Bad,
    onError = Tokens.Bg,
    errorContainer = Tokens.Bg,
    onErrorContainer = Tokens.Bad,
    scrim = Color.Black,
)

private val AppTypography = Typography(
    headlineMedium = TextStyle(fontFamily = Sans, fontWeight = FontWeight.SemiBold, fontSize = 24.sp, lineHeight = 30.sp, letterSpacing = (-0.2).sp),
    headlineSmall = TextStyle(fontFamily = Sans, fontWeight = FontWeight.Medium, fontSize = 26.sp, lineHeight = 30.sp, letterSpacing = (-0.3).sp),
    titleLarge = TextStyle(fontFamily = Sans, fontWeight = FontWeight.SemiBold, fontSize = 20.sp, lineHeight = 26.sp),
    titleMedium = TextStyle(fontFamily = Sans, fontWeight = FontWeight.Medium, fontSize = 15.sp, lineHeight = 22.sp),
    titleSmall = TextStyle(fontFamily = Sans, fontWeight = FontWeight.Medium, fontSize = 15.sp, lineHeight = 22.sp),
    bodyLarge = TextStyle(fontFamily = Sans, fontWeight = FontWeight.Normal, fontSize = 17.sp, lineHeight = 26.sp),
    bodyMedium = TextStyle(fontFamily = Sans, fontWeight = FontWeight.Normal, fontSize = 15.sp, lineHeight = 23.sp),
    bodySmall = TextStyle(fontFamily = Sans, fontWeight = FontWeight.Normal, fontSize = 13.5.sp, lineHeight = 20.sp),
    labelLarge = TextStyle(fontFamily = Sans, fontWeight = FontWeight.Medium, fontSize = 14.sp, lineHeight = 20.sp),
    labelMedium = TextStyle(fontFamily = Mono, fontWeight = FontWeight.Normal, fontSize = 12.5.sp, lineHeight = 18.sp),
    labelSmall = TextStyle(fontFamily = Mono, fontWeight = FontWeight.Normal, fontSize = 12.sp, lineHeight = 16.sp, letterSpacing = 0.2.sp),
)

private val Square = RoundedCornerShape(0.dp)
private val AppShapes = Shapes(extraSmall = Square, small = Square, medium = Square, large = Square, extraLarge = Square)

@Composable
fun SimhookTheme(content: @Composable () -> Unit) {
    MaterialTheme(colorScheme = LightColors, typography = AppTypography, shapes = AppShapes, content = content)
}
