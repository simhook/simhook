package dev.simhook.app.ui.home

import androidx.compose.foundation.background
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.shape.CircleShape
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Card
import androidx.compose.material3.CardDefaults
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.Switch
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import dev.simhook.app.push.Push
import dev.simhook.app.sms.SimInfo
import dev.simhook.app.ui.Permissions
import dev.simhook.app.ui.relativeTime

@Composable
fun DashboardScreen(vm: GatewayViewModel, permissionsOk: Boolean, modifier: Modifier = Modifier) {
    val context = LocalContext.current
    val settings by vm.settings.collectAsStateWithLifecycle()
    val queued by vm.queued.collectAsStateWithLifecycle()
    val busy by vm.busy.collectAsStateWithLifecycle()
    val pushAvailable = remember { Push.available(context) }
    val sims = remember(settings.deviceId) { SimInfo.list(context) }

    // Settings changed from the dashboard should show up as soon as the app is opened.
    LaunchedEffect(Unit) { vm.refresh() }

    Column(
        modifier = modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState())
            .padding(16.dp),
        verticalArrangement = Arrangement.spacedBy(12.dp),
    ) {
        Card(modifier = Modifier.fillMaxWidth()) {
            Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
                Row(verticalAlignment = Alignment.CenterVertically, horizontalArrangement = Arrangement.spacedBy(10.dp)) {
                    Box(
                        Modifier
                            .size(12.dp)
                            .background(if (settings.online) MaterialTheme.colorScheme.primary else MaterialTheme.colorScheme.outline, CircleShape),
                    )
                    Text(settings.deviceName.ifBlank { "This phone" }, style = MaterialTheme.typography.titleLarge)
                }
                Text(
                    if (settings.online) "Online" else "Offline as far as the server knows",
                    style = MaterialTheme.typography.bodyMedium,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
                Text("Last check-in: ${relativeTime(settings.lastHeartbeatAt)}", style = MaterialTheme.typography.bodyMedium)
                Row(horizontalArrangement = Arrangement.spacedBy(8.dp)) {
                    OutlinedButton(onClick = vm::checkInNow, enabled = !busy) { Text("Check in now") }
                    TextButton(onClick = vm::refresh, enabled = !busy) { Text("Refresh") }
                }
            }
        }

        if (!permissionsOk) {
            WarningCard(
                title = "SMS permission missing",
                text = "The gateway cannot send or receive until SMS permissions are granted.",
                action = "Open settings",
                onAction = { Permissions.openAppSettings(context) },
            )
        }
        if (!pushAvailable) {
            WarningCard(
                title = "Push not configured in this build",
                text = "This build has no push configuration, so the server cannot wake the phone. Sends will not arrive.",
            )
        } else if (settings.pushToken == null) {
            WarningCard(
                title = "Waiting for push registration",
                text = "The phone has not registered for pushes yet. It usually takes a moment after pairing.",
            )
        }

        Row(horizontalArrangement = Arrangement.spacedBy(12.dp)) {
            StatCard("Sent", settings.sentCount.toString(), Modifier.weight(1f))
            StatCard("Received", settings.receivedCount.toString(), Modifier.weight(1f))
            StatCard("Queued", queued.toString(), Modifier.weight(1f))
        }

        Card(modifier = Modifier.fillMaxWidth()) {
            Column(Modifier.padding(horizontal = 16.dp, vertical = 4.dp)) {
                ToggleRow(
                    title = "Gateway enabled",
                    subtitle = "When off, the server holds sends for another device.",
                    checked = settings.gatewayEnabled,
                    enabled = !busy,
                    onChange = vm::setGatewayEnabled,
                )
                ToggleRow(
                    title = "Forward incoming SMS",
                    subtitle = "Report messages this phone receives to your webhooks.",
                    checked = settings.receiveEnabled,
                    enabled = !busy,
                    onChange = vm::setReceiveEnabled,
                )
            }
        }

        Card(modifier = Modifier.fillMaxWidth()) {
            Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
                Text("SIM cards", style = MaterialTheme.typography.titleMedium)
                if (sims.isEmpty()) {
                    Text(
                        if (SimInfo.hasPermission(context)) "No active SIM found." else "Grant the phone state permission to list SIMs.",
                        style = MaterialTheme.typography.bodyMedium,
                        color = MaterialTheme.colorScheme.onSurfaceVariant,
                    )
                }
                sims.forEach { sim ->
                    val name = sim.displayName ?: sim.carrier ?: "SIM ${sim.slot + 1}"
                    val preferred = settings.preferredSimSubscriptionId == sim.subscriptionId
                    Text(
                        "$name  ·  id ${sim.subscriptionId}" + if (preferred) "  ·  preferred" else "",
                        style = MaterialTheme.typography.bodyMedium,
                    )
                }
                Text(
                    "Pass a SIM id as sim_subscription_id in an API call to send from that SIM.",
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant,
                )
            }
        }
    }
}

@Composable
private fun StatCard(label: String, value: String, modifier: Modifier = Modifier) {
    Card(modifier = modifier) {
        Column(Modifier.padding(12.dp)) {
            Text(value, style = MaterialTheme.typography.headlineSmall)
            Text(label, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
    }
}

@Composable
fun ToggleRow(title: String, subtitle: String?, checked: Boolean, enabled: Boolean, onChange: (Boolean) -> Unit) {
    Row(
        modifier = Modifier
            .fillMaxWidth()
            .padding(vertical = 8.dp),
        verticalAlignment = Alignment.CenterVertically,
    ) {
        Column(Modifier.weight(1f)) {
            Text(title, style = MaterialTheme.typography.titleMedium)
            if (subtitle != null) Text(subtitle, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }
        Switch(checked = checked, onCheckedChange = onChange, enabled = enabled)
    }
}

@Composable
fun WarningCard(title: String, text: String, action: String? = null, onAction: (() -> Unit)? = null) {
    Card(
        modifier = Modifier.fillMaxWidth(),
        colors = CardDefaults.cardColors(containerColor = MaterialTheme.colorScheme.errorContainer),
    ) {
        Column(Modifier.padding(16.dp), verticalArrangement = Arrangement.spacedBy(4.dp)) {
            Text(title, style = MaterialTheme.typography.titleMedium, color = MaterialTheme.colorScheme.onErrorContainer)
            Text(text, style = MaterialTheme.typography.bodyMedium, color = MaterialTheme.colorScheme.onErrorContainer)
            if (action != null && onAction != null) {
                TextButton(onClick = onAction) { Text(action) }
            }
        }
    }
}
