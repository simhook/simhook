package dev.simhook.app.ui.home

import androidx.compose.foundation.BorderStroke
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableFloatStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.graphics.RectangleShape
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import dev.simhook.app.BuildConfig
import dev.simhook.app.push.Push
import dev.simhook.app.ui.Permissions
import dev.simhook.app.ui.rememberSims
import dev.simhook.app.ui.theme.FilledButton
import dev.simhook.app.ui.theme.Hairline
import dev.simhook.app.ui.theme.ListRow
import dev.simhook.app.ui.theme.PlainSlider
import dev.simhook.app.ui.theme.PlainSwitch
import dev.simhook.app.ui.theme.PlainTextField
import dev.simhook.app.ui.theme.Section
import dev.simhook.app.ui.theme.TextLink
import dev.simhook.app.ui.theme.Tokens
import dev.simhook.app.ui.theme.Tone

@Composable
fun SettingsScreen(vm: GatewayViewModel, modifier: Modifier = Modifier) {
    val context = LocalContext.current
    val settings by vm.settings.collectAsStateWithLifecycle()
    val busy by vm.busy.collectAsStateWithLifecycle()
    val sims = rememberSims()
    var renaming by remember { mutableStateOf(false) }
    var confirmUnpair by remember { mutableStateOf(false) }
    var simMenu by remember { mutableStateOf(false) }
    var delay by remember(settings.sendDelaySeconds) { mutableFloatStateOf(settings.sendDelaySeconds.toFloat()) }
    val update = settings.update?.takeIf { it.versionCode > BuildConfig.VERSION_CODE }

    Column(
        modifier = modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState())
            .padding(horizontal = 20.dp, vertical = 24.dp),
        verticalArrangement = Arrangement.spacedBy(28.dp),
    ) {
        Section("this phone") {
            ListRow("Name", settings.deviceName.ifBlank { "Unnamed" }, onClick = { renaming = true })
            Box {
                val preferred = sims.firstOrNull { it.subscriptionId == settings.preferredSimSubscriptionId }
                ListRow(
                    "Preferred SIM",
                    preferred?.let { it.displayName ?: it.carrier ?: "SIM ${it.slot + 1}" } ?: "Phone default",
                    onClick = { simMenu = true },
                )
                DropdownMenu(
                    expanded = simMenu,
                    onDismissRequest = { simMenu = false },
                    shape = RectangleShape,
                    containerColor = Tokens.Bg,
                    shadowElevation = 0.dp,
                    border = BorderStroke(1.dp, Tokens.Line),
                ) {
                    DropdownMenuItem(text = { Text("Phone default") }, onClick = { simMenu = false; vm.setPreferredSim(null) })
                    sims.forEach { sim ->
                        DropdownMenuItem(
                            text = { Text((sim.displayName ?: sim.carrier ?: "SIM ${sim.slot + 1}") + ", id ${sim.subscriptionId}") },
                            onClick = { simMenu = false; vm.setPreferredSim(sim.subscriptionId) },
                        )
                    }
                }
            }
            ListRow(
                title = "Forward incoming SMS",
                trailing = { PlainSwitch(settings.receiveEnabled, vm::setReceiveEnabled, enabled = !busy) },
            )
        }

        Section("sending") {
            Column(Modifier.padding(top = 10.dp)) {
                Text("Delay between sends: ${delay.toInt()} s", style = MaterialTheme.typography.bodyMedium)
                Text(
                    "A pause after each message keeps the carrier from flagging the SIM. Five seconds is a sensible default.",
                    style = MaterialTheme.typography.bodySmall,
                    color = Tokens.Muted,
                )
                PlainSlider(
                    value = delay,
                    onValueChange = { delay = it },
                    onValueChangeFinished = { vm.setSendDelay(delay.toInt()) },
                    valueRange = 0f..60f,
                    steps = 59,
                    enabled = !busy,
                )
            }
            Hairline()
        }

        Section("staying awake") {
            ListRow(
                title = "Keep-alive notification",
                subtitle = "Keeps a quiet notification up so the phone never freezes the app. Turn it on if sends stop arriving when the screen is off.",
                trailing = { PlainSwitch(settings.keepAliveNotification, vm::setKeepAlive) },
            )
            if (!Permissions.ignoringBatteryOptimizations(context)) {
                ListRow(
                    title = "Battery optimisation",
                    subtitle = "Android may pause the app in the background.",
                    trailing = { TextLink("Allow", onClick = { Permissions.requestIgnoreBatteryOptimizations(context) }) },
                )
            }
        }

        Section("server") {
            ListRow("Check-in interval", "Every ${settings.heartbeatIntervalMinutes} minutes, set from the dashboard")
            ListRow("Server", settings.apiBaseUrl)
            ListRow("Push", if (!Push.available(context)) "Not configured in this build" else if (settings.pushToken != null) "Registered" else "Not registered yet")
            ListRow(
                "App version",
                if (update != null) "${BuildConfig.VERSION_NAME}, ${update.versionName} available" else BuildConfig.VERSION_NAME,
                trailing = {
                    if (update != null) TextLink("Install", onClick = vm::installUpdate) else TextLink("Check for updates", onClick = vm::checkForUpdates)
                },
            )
        }

        Column(verticalArrangement = Arrangement.spacedBy(4.dp)) {
            TextLink("Unpair this phone", onClick = { confirmUnpair = true }, tone = Tone.Bad)
            Text(
                "Unpairing removes this phone from your account. Message history stays in the dashboard.",
                style = MaterialTheme.typography.bodySmall,
                color = Tokens.Muted,
            )
        }
    }

    if (renaming) {
        var name by remember { mutableStateOf(settings.deviceName) }
        PlainDialog(
            title = "Name",
            onDismiss = { renaming = false },
            confirm = { FilledButton("Save", onClick = { renaming = false; if (name.isNotBlank()) vm.rename(name) }) },
        ) {
            PlainTextField(value = name, onValueChange = { name = it.take(64) }, label = "Name", modifier = Modifier.fillMaxWidth())
        }
    }
    if (confirmUnpair) {
        PlainDialog(
            title = "Unpair this phone?",
            onDismiss = { confirmUnpair = false },
            confirm = { FilledButton("Unpair", onClick = { confirmUnpair = false; vm.unpair() }) },
        ) {
            Text("Messages waiting on this phone are discarded. You can pair again with a new code.", style = MaterialTheme.typography.bodyMedium, color = Tokens.Muted)
        }
    }
}

/** A dialog is a white rectangle with a hairline: a title, some words, one filled button, and a way out. */
@Composable
fun PlainDialog(title: String, onDismiss: () -> Unit, confirm: @Composable () -> Unit, content: @Composable () -> Unit) {
    AlertDialog(
        onDismissRequest = onDismiss,
        shape = RectangleShape,
        containerColor = Tokens.Bg,
        titleContentColor = Tokens.Fg,
        textContentColor = Tokens.Fg,
        tonalElevation = 0.dp,
        title = { Text(title, style = MaterialTheme.typography.titleLarge) },
        text = { content() },
        confirmButton = confirm,
        dismissButton = { TextLink("Cancel", onClick = onDismiss) },
    )
}
