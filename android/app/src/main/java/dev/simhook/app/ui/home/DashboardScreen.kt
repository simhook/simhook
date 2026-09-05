package dev.simhook.app.ui.home

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.remember
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import dev.simhook.app.BuildConfig
import dev.simhook.app.core.AvailableUpdate
import dev.simhook.app.push.Push
import dev.simhook.app.ui.Permissions
import dev.simhook.app.ui.formatBytes
import dev.simhook.app.ui.relativeTime
import dev.simhook.app.ui.rememberSims
import dev.simhook.app.ui.theme.FilledButton
import dev.simhook.app.ui.theme.Hairline
import dev.simhook.app.ui.theme.ListRow
import dev.simhook.app.ui.theme.Note
import dev.simhook.app.ui.theme.PlainSwitch
import dev.simhook.app.ui.theme.Section
import dev.simhook.app.ui.theme.Stat
import dev.simhook.app.ui.theme.StatusWord
import dev.simhook.app.ui.theme.TextLink
import dev.simhook.app.ui.theme.Tokens
import dev.simhook.app.ui.theme.Tone

@Composable
fun DashboardScreen(vm: GatewayViewModel, permissionsOk: Boolean, modifier: Modifier = Modifier) {
    val context = LocalContext.current
    val settings by vm.settings.collectAsStateWithLifecycle()
    val queued by vm.queued.collectAsStateWithLifecycle()
    val busy by vm.busy.collectAsStateWithLifecycle()
    val pushAvailable = remember { Push.available(context) }
    val sims = rememberSims()
    val update = settings.update?.takeIf { it.versionCode > BuildConfig.VERSION_CODE }

    // Settings changed from the dashboard should show up as soon as the app is opened,
    // and a newer build should be offered without waiting for the background check.
    LaunchedEffect(Unit) {
        vm.refresh()
        vm.checkForUpdatesQuietly()
    }

    Column(
        modifier = modifier
            .fillMaxSize()
            .verticalScroll(rememberScrollState())
            .padding(horizontal = 20.dp, vertical = 24.dp),
        verticalArrangement = Arrangement.spacedBy(28.dp),
    ) {
        Column(verticalArrangement = Arrangement.spacedBy(6.dp)) {
            Text(settings.deviceName.ifBlank { "This phone" }, style = MaterialTheme.typography.headlineMedium)
            StatusWord(if (settings.online) Tone.Ok else Tone.Off, if (settings.online) "Online" else "Offline, as far as the server knows")
            Text("Last check-in ${relativeTime(settings.lastHeartbeatAt)}", style = MaterialTheme.typography.bodySmall, color = Tokens.Muted)
            Row(horizontalArrangement = Arrangement.spacedBy(20.dp)) {
                TextLink("Check in now", onClick = vm::checkInNow, enabled = !busy)
                TextLink("Refresh", onClick = vm::refresh, enabled = !busy)
            }
        }

        if (update != null) {
            UpdateSection(
                update,
                downloading = settings.updateDownloadId >= 0,
                ready = settings.updateReadyUri != null,
                onUpdate = vm::installUpdate,
            )
        }

        if (!permissionsOk || !pushAvailable || settings.pushToken == null) {
            Column(verticalArrangement = Arrangement.spacedBy(16.dp)) {
                if (!permissionsOk) {
                    Note(
                        title = "SMS permission missing",
                        text = "Nothing can be sent or received until the SMS permissions are granted.",
                        action = { TextLink("Open settings", onClick = { Permissions.openAppSettings(context) }) },
                    )
                }
                if (!pushAvailable) {
                    Note(
                        title = "Push not configured in this build",
                        text = "The server cannot wake this phone, so sends arrive only at each check-in.",
                        tone = Tone.Warn,
                    )
                } else if (settings.pushToken == null) {
                    Note(
                        title = "Waiting for push registration",
                        text = "The phone has not registered for pushes yet. It usually takes a moment after pairing.",
                        tone = Tone.Warn,
                    )
                }
            }
        }

        Row(horizontalArrangement = Arrangement.spacedBy(28.dp)) {
            Stat(settings.sentCount.toString(), "sent")
            Stat(settings.receivedCount.toString(), "received")
            Stat(queued.toString(), "queued")
        }

        Section("gateway") {
            ListRow(
                title = "Sending",
                subtitle = "When off, this phone sends nothing; sends go to another phone or wait.",
                trailing = { PlainSwitch(settings.gatewayEnabled, vm::setGatewayEnabled, enabled = !busy) },
            )
            ListRow(
                title = "Forward incoming SMS",
                subtitle = "Texts this phone receives go to your account and your webhooks.",
                trailing = { PlainSwitch(settings.receiveEnabled, vm::setReceiveEnabled, enabled = !busy) },
            )
        }

        Section("sim cards") {
            if (sims.isEmpty()) {
                Text(
                    if (dev.simhook.app.sms.SimInfo.hasPermission(context)) "No active SIM found." else "Grant the phone state permission to list SIMs.",
                    style = MaterialTheme.typography.bodyMedium,
                    color = Tokens.Muted,
                    modifier = Modifier.padding(vertical = 10.dp),
                )
                Hairline()
            }
            sims.forEach { sim ->
                val name = sim.displayName ?: sim.carrier ?: "SIM ${sim.slot + 1}"
                val preferred = settings.preferredSimSubscriptionId == sim.subscriptionId
                ListRow(
                    title = name,
                    subtitle = "id ${sim.subscriptionId}" + if (preferred) ", preferred" else "",
                )
            }
            Spacer(Modifier.height(8.dp))
            Text(
                "Pass a SIM id as sim_subscription_id in an API call to send from that SIM.",
                style = MaterialTheme.typography.bodySmall,
                color = Tokens.Muted,
            )
        }
    }
}

@Composable
private fun UpdateSection(update: AvailableUpdate, downloading: Boolean, ready: Boolean, onUpdate: () -> Unit) {
    Section(if (update.required) "update required" else "update available") {
        Column(Modifier.padding(top = 10.dp), verticalArrangement = Arrangement.spacedBy(6.dp)) {
            Text(
                "simhook ${update.versionName}" + if (update.sizeBytes > 0) ", ${formatBytes(update.sizeBytes)}" else "",
                style = MaterialTheme.typography.titleMedium,
            )
            if (update.required) {
                Text(
                    "The server no longer supports this version. Sending and forwarding may stop until you update.",
                    style = MaterialTheme.typography.bodySmall,
                    color = Tokens.Muted,
                )
            }
            update.notes?.let { Text(it, style = MaterialTheme.typography.bodySmall, color = Tokens.Muted) }
            if (ready && !downloading) {
                StatusWord(Tone.Ok, "Downloaded and verified", muted = true)
            }
            Row(Modifier.padding(top = 6.dp), verticalAlignment = Alignment.CenterVertically) {
                FilledButton(
                    when {
                        downloading -> "Downloading…"
                        ready -> "Install"
                        else -> "Download and install"
                    },
                    onClick = onUpdate,
                    enabled = !downloading,
                )
            }
        }
    }
}
