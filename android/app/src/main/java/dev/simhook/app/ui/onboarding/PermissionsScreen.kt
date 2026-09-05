package dev.simhook.app.ui.onboarding

import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.result.contract.ActivityResultContracts
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.safeDrawingPadding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import androidx.lifecycle.compose.LocalLifecycleOwner
import dev.simhook.app.ui.Permissions
import dev.simhook.app.ui.theme.FilledButton
import dev.simhook.app.ui.theme.Hairline
import dev.simhook.app.ui.theme.Mono
import dev.simhook.app.ui.theme.StatusWord
import dev.simhook.app.ui.theme.TextLink
import dev.simhook.app.ui.theme.Tokens
import dev.simhook.app.ui.theme.Tone

@Composable
fun PermissionsScreen(onDone: () -> Unit) {
    val context = LocalContext.current
    val lifecycleOwner = LocalLifecycleOwner.current
    var granted by remember { mutableStateOf(Permissions.items.associate { it.permission to Permissions.granted(context, it.permission) }) }
    var batteryOk by remember { mutableStateOf(Permissions.ignoringBatteryOptimizations(context)) }
    val launcher = rememberLauncherForActivityResult(ActivityResultContracts.RequestMultiplePermissions()) {
        granted = Permissions.items.associate { it.permission to Permissions.granted(context, it.permission) }
    }

    DisposableEffect(lifecycleOwner) {
        val observer = LifecycleEventObserver { _, event ->
            if (event == Lifecycle.Event.ON_RESUME) {
                granted = Permissions.items.associate { it.permission to Permissions.granted(context, it.permission) }
                batteryOk = Permissions.ignoringBatteryOptimizations(context)
            }
        }
        lifecycleOwner.lifecycle.addObserver(observer)
        onDispose { lifecycleOwner.lifecycle.removeObserver(observer) }
    }

    val coreOk = Permissions.items.filter { it.required }.all { granted[it.permission] == true }
    val allOk = Permissions.items.all { granted[it.permission] == true }

    Column(
        modifier = Modifier
            .fillMaxSize()
            .safeDrawingPadding()
            .verticalScroll(rememberScrollState())
            .padding(horizontal = 20.dp, vertical = 24.dp),
        verticalArrangement = Arrangement.spacedBy(16.dp),
    ) {
        Text("simhook", fontFamily = Mono, fontWeight = FontWeight.Medium, style = MaterialTheme.typography.titleMedium)
        Spacer(Modifier.height(8.dp))
        Text("Permissions", style = MaterialTheme.typography.headlineMedium)
        Text(
            "The app needs to send and receive SMS. The rest is optional but worth granting.",
            style = MaterialTheme.typography.bodyMedium,
            color = Tokens.Muted,
        )
        Column {
            Hairline()
            Permissions.items.forEach { item ->
                val ok = granted[item.permission] == true
                Column(Modifier.padding(vertical = 10.dp), verticalArrangement = Arrangement.spacedBy(2.dp)) {
                    StatusWord(if (ok) Tone.Ok else Tone.Off, item.title + if (item.required) "" else " (optional)")
                    Text(item.reason, style = MaterialTheme.typography.bodySmall, color = Tokens.Muted)
                }
                Hairline()
            }
            Column(Modifier.padding(vertical = 10.dp), verticalArrangement = Arrangement.spacedBy(2.dp)) {
                StatusWord(if (batteryOk) Tone.Ok else Tone.Off, "Background activity (recommended)")
                Text(
                    "Let the app run in the background so messages go out while the screen is off.",
                    style = MaterialTheme.typography.bodySmall,
                    color = Tokens.Muted,
                )
                if (!batteryOk) TextLink("Allow background activity", onClick = { Permissions.requestIgnoreBatteryOptimizations(context) })
            }
            Hairline()
        }
        // One filled button: granting until the app can work, then continuing.
        if (!coreOk) {
            FilledButton("Grant permissions", onClick = { launcher.launch(Permissions.all()) }, modifier = Modifier.fillMaxWidth())
        } else {
            if (!allOk) TextLink("Grant the optional ones too", onClick = { launcher.launch(Permissions.all()) })
            FilledButton("Continue", onClick = onDone, modifier = Modifier.fillMaxWidth())
        }
    }
}
