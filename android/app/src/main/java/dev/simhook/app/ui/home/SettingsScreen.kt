package dev.simhook.app.ui.home

import androidx.compose.foundation.clickable
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Button
import androidx.compose.material3.Card
import androidx.compose.material3.DropdownMenu
import androidx.compose.material3.DropdownMenuItem
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Slider
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableFloatStateOf
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import dev.simhook.app.BuildConfig
import dev.simhook.app.push.Push
import dev.simhook.app.sms.SimInfo
import dev.simhook.app.ui.Permissions

@Composable
fun SettingsScreen(vm: GatewayViewModel, modifier: Modifier = Modifier) {
    val context = LocalContext.current
    val settings by vm.settings.collectAsStateWithLifecycle()
    val busy by vm.busy.collectAsStateWithLifecycle()
    val sims = remember(settings.deviceId) { SimInfo.list(context) }
    var renaming by remember { mutableStateOf(false) }
    var confirmUnpair by remember { mutableStateOf(false) }
    var simMenu by remember { mutableStateOf(false) }
    var delay by remember(settings.sendDelaySeconds) { mutableFloatStateOf(settings.sendDelaySeconds.toFloat()) }

    Column(
        modifier = modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState())
            .padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        Text("Settings", style = MaterialTheme.typography.headlineSmall)

        Card(Modifier.fillMaxWidth()) {
            Column(Modifier.padding(horizontal = 16.dp, vertical = 4.dp)) {
                InfoRow("Device name", settings.deviceName.ifBlank { "Unnamed" }, onClick = { renaming = true })
                HorizontalDivider()
                Column(Modifier.padding(vertical = 8.dp)) {
                    Text("Delay between sends: ${delay.toInt()} s", style = MaterialTheme.typography.titleMedium)
                    Text(
                        "A pause after each message keeps the carrier from flagging the SIM. Five seconds is a sensible default.",
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                    Slider(
                        value = delay,
                        onValueChange = { delay = it },
                        onValueChangeFinished = { vm.setSendDelay(delay.toInt()) },
                        valueRange = 0f..60f,
                        steps = 59,
                        enabled = !busy,
                    )
                }
                HorizontalDivider()
                Column(Modifier.padding(vertical = 8.dp)) {
                    val preferred = sims.firstOrNull { it.subscriptionId == settings.preferredSimSubscriptionId }
                    InfoRow(
                        "Preferred SIM",
                        preferred?.let { it.displayName ?: it.carrier ?: "SIM ${it.slot + 1}" } ?: "Phone default",
                        onClick = { simMenu = true },
                    )
                    DropdownMenu(expanded = simMenu, onDismissRequest = { simMenu = false }) {
                        DropdownMenuItem(text = { Text("Phone default") }, onClick = { simMenu = false; vm.setPreferredSim(null) })
                        sims.forEach { sim ->
                            DropdownMenuItem(
                                text = { Text((sim.displayName ?: sim.carrier ?: "SIM ${sim.slot + 1}") + "  ·  id ${sim.subscriptionId}") },
                                onClick = { simMenu = false; vm.setPreferredSim(sim.subscriptionId) },
                            )
                        }
                    }
                }
                HorizontalDivider()
                ToggleRow(
                    title = "Forward incoming SMS",
                    subtitle = null,
                    checked = settings.receiveEnabled,
                    enabled = !busy,
                    onChange = vm::setReceiveEnabled,
                )
            }
        }

        Card(Modifier.fillMaxWidth()) {
            Column(Modifier.padding(horizontal = 16.dp, vertical = 4.dp)) {
                ToggleRow(
                    title = "Keep-alive notification",
                    subtitle = "Keeps a quiet notification up so the phone never freezes the gateway. Turn on if sends stop arriving when the screen is off.",
                    checked = settings.keepAliveNotification,
                    enabled = true,
                    onChange = vm::setKeepAlive,
                )
                if (!Permissions.ignoringBatteryOptimizations(context)) {
                    HorizontalDivider()
                    Row(Modifier.padding(vertical = 8.dp), verticalAlignment = Alignment.CenterVertically) {
                        Column(Modifier.weight(1f)) {
                            Text("Battery optimization", style = MaterialTheme.typography.titleMedium)
                            Text("Android may pause the app in the background.", style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                        }
                        OutlinedButton(onClick = { Permissions.requestIgnoreBatteryOptimizations(context) }) { Text("Allow") }
                    }
                }
            }
        }

        Card(Modifier.fillMaxWidth()) {
            Column(Modifier.padding(horizontal = 16.dp, vertical = 4.dp)) {
                InfoRow("Check-in interval", "Every ${settings.heartbeatIntervalMinutes} minutes, set from the dashboard")
                HorizontalDivider()
                InfoRow("Server", settings.apiBaseUrl)
                HorizontalDivider()
                InfoRow("Push", if (!Push.available(context)) "Not configured in this build" else if (settings.pushToken != null) "Registered" else "Not registered yet")
                HorizontalDivider()
                val update = settings.update?.takeIf { it.versionCode > BuildConfig.VERSION_CODE }
                InfoRow(
                    "App version",
                    when {
                        update != null -> "${BuildConfig.VERSION_NAME}  ·  ${update.versionName} available, tap to install"
                        else -> "${BuildConfig.VERSION_NAME}  ·  tap to check for updates"
                    },
                    onClick = { if (update != null) vm.installUpdate() else vm.checkForUpdates() },
                )
            }
        }

        OutlinedButton(onClick = { confirmUnpair = true }, modifier = Modifier.fillMaxWidth()) { Text("Unpair this phone") }
        Text(
            "Unpairing removes this phone from your account. Message history stays in the dashboard.",
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
    }

    if (renaming) {
        var name by remember { mutableStateOf(settings.deviceName) }
        AlertDialog(
            onDismissRequest = { renaming = false },
            title = { Text("Device name") },
            text = { OutlinedTextField(value = name, onValueChange = { name = it.take(64) }, singleLine = true) },
            confirmButton = {
                Button(onClick = { renaming = false; if (name.isNotBlank()) vm.rename(name) }) { Text("Save") }
            },
            dismissButton = { TextButton(onClick = { renaming = false }) { Text("Cancel") } },
        )
    }
    if (confirmUnpair) {
        AlertDialog(
            onDismissRequest = { confirmUnpair = false },
            title = { Text("Unpair this phone?") },
            text = { Text("Queued messages on this phone are discarded. You can pair again with a new code.") },
            confirmButton = { Button(onClick = { confirmUnpair = false; vm.unpair() }) { Text("Unpair") } },
            dismissButton = { TextButton(onClick = { confirmUnpair = false }) { Text("Cancel") } },
        )
    }
}

@Composable
private fun InfoRow(title: String, value: String, onClick: (() -> Unit)? = null) {
    Column(
        Modifier
            .fillMaxWidth()
            .then(if (onClick != null) Modifier.clickable(onClick = onClick) else Modifier)
            .padding(vertical = 10.dp),
    ) {
        Text(title, style = MaterialTheme.typography.titleMedium)
        Text(value, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
    }
}
