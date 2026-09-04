package dev.simhook.app.ui.theme

import android.os.Build
import androidx.compose.foundation.isSystemInDarkTheme
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.darkColorScheme
import androidx.compose.material3.dynamicDarkColorScheme
import androidx.compose.material3.dynamicLightColorScheme
import androidx.compose.material3.lightColorScheme
import androidx.compose.runtime.Composable
import androidx.compose.ui.graphics.Color
import androidx.compose.ui.platform.LocalContext

private val LightColors = lightColorScheme(
    primary = Color(0xFF0E7C86),
    onPrimary = Color.White,
    primaryContainer = Color(0xFFB2EBF2),
    onPrimaryContainer = Color(0xFF00363B),
    secondary = Color(0xFF4A6367),
    tertiary = Color(0xFF8F4E00),
)

private val DarkColors = darkColorScheme(
    primary = Color(0xFF7BD6E0),
    onPrimary = Color(0xFF003A40),
    primaryContainer = Color(0xFF00545C),
    onPrimaryContainer = Color(0xFFB2EBF2),
    secondary = Color(0xFFB1CBCF),
    tertiary = Color(0xFFFFB877),
)

@Composable
fun SimhookTheme(content: @Composable () -> Unit) {
    val dark = isSystemInDarkTheme()
    val context = LocalContext.current
    val scheme = when {
        Build.VERSION.SDK_INT >= Build.VERSION_CODES.S -> if (dark) dynamicDarkColorScheme(context) else dynamicLightColorScheme(context)
        dark -> DarkColors
        else -> LightColors
    }
    MaterialTheme(colorScheme = scheme, content = content)
}
