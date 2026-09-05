package dev.simhook.app.ui.onboarding

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.safeDrawingPadding
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.KeyboardCapitalization
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import com.google.mlkit.vision.barcode.common.Barcode
import com.google.mlkit.vision.codescanner.GmsBarcodeScannerOptions
import com.google.mlkit.vision.codescanner.GmsBarcodeScanning
import dev.simhook.app.AppContainer
import dev.simhook.app.api.ApiException
import dev.simhook.app.core.AppSettings
import dev.simhook.app.core.PairLink
import dev.simhook.app.push.Push
import dev.simhook.app.ui.theme.FilledButton
import dev.simhook.app.ui.theme.Mono
import dev.simhook.app.ui.theme.Note
import dev.simhook.app.ui.theme.PlainTextField
import dev.simhook.app.ui.theme.TextLink
import dev.simhook.app.ui.theme.Tokens
import dev.simhook.app.ui.theme.Tone
import kotlinx.coroutines.launch

@Composable
fun PairScreen(container: AppContainer, link: PairLink?, onLinkConsumed: () -> Unit) {
    val context = LocalContext.current
    val scope = rememberCoroutineScope()
    var code by rememberSaveable { mutableStateOf("") }
    var apiUrl by rememberSaveable { mutableStateOf(AppSettings.DEFAULT_API_URL) }
    var showServer by rememberSaveable { mutableStateOf(false) }
    var busy by remember { mutableStateOf(false) }
    var error by remember { mutableStateOf<String?>(null) }
    // A link or code that names a server other than the default waits here
    // for a tap: the phone would otherwise hand its credentials to whatever
    // the link said.
    var foreign by remember { mutableStateOf<PairLink?>(null) }

    fun pair(withCode: String = code, withApi: String = apiUrl) {
        if (busy) return
        val api = withApi.trim().trimEnd('/')
        if (!PairLink.allowedApi(api)) {
            error = "The server address must start with https://."
            return
        }
        scope.launch {
            busy = true
            error = null
            try {
                val token = Push.token(context)
                container.pair(withCode.trim(), api, token)
            } catch (e: ApiException) {
                error = e.message
            } catch (e: Exception) {
                error = "Could not reach the server. Check the address and your connection. (${e.javaClass.simpleName}: ${e.message})"
            } finally {
                busy = false
            }
        }
    }

    // A link or scanned code carries everything needed. With the default
    // server it pairs at once; with another server it asks first.
    fun applyLink(parsed: PairLink) {
        code = parsed.code
        error = null
        val api = parsed.api
        if (api != null && api != AppSettings.DEFAULT_API_URL) {
            apiUrl = api
            showServer = true
            foreign = parsed
            return
        }
        pair(parsed.code, api ?: apiUrl)
    }

    LaunchedEffect(link) {
        if (link != null) {
            onLinkConsumed()
            applyLink(link)
        }
    }

    fun scan() {
        val options = GmsBarcodeScannerOptions.Builder().setBarcodeFormats(Barcode.FORMAT_QR_CODE).build()
        GmsBarcodeScanning.getClient(context, options).startScan()
            .addOnSuccessListener { barcode ->
                val parsed = barcode.rawValue?.let(PairLink::parse)
                if (parsed == null) error = "That QR code is not a pairing code." else applyLink(parsed)
            }
            .addOnFailureListener { error = "The scanner could not open. Type the code instead." }
    }

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
        Text("Pair this phone", style = MaterialTheme.typography.headlineMedium)
        Text(
            "In the dashboard, open Phones and choose Pair a phone. Scan the code, or type it below. The code works once and expires in ten minutes.",
            style = MaterialTheme.typography.bodyMedium,
            color = Tokens.Muted,
        )
        PlainTextField(
            value = code,
            onValueChange = { code = it.uppercase().take(9) },
            label = "Pairing code",
            placeholder = "ABCD-EFGH",
            keyboardOptions = KeyboardOptions(capitalization = KeyboardCapitalization.Characters, keyboardType = KeyboardType.Ascii),
            modifier = Modifier.fillMaxWidth(),
        )
        Row(horizontalArrangement = Arrangement.spacedBy(20.dp)) {
            TextLink("Scan a QR code", onClick = ::scan, enabled = !busy)
            if (!showServer) TextLink("Use a different server", onClick = { showServer = true })
        }
        if (showServer) {
            PlainTextField(
                value = apiUrl,
                onValueChange = { apiUrl = it; foreign = null },
                label = "Server address",
                keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Uri),
                modifier = Modifier.fillMaxWidth(),
            )
        }
        error?.let { Note(title = it, tone = Tone.Bad) }
        val pending = foreign
        if (pending != null) {
            Note(
                title = "This code pairs with ${pending.host ?: pending.api}",
                text = "Only pair with a server you run or trust: it will hold this phone's credentials and every message.",
                tone = Tone.Off,
            )
            Row(horizontalArrangement = Arrangement.spacedBy(20.dp)) {
                FilledButton("Pair with ${pending.host ?: "this server"}", onClick = { foreign = null; pair(pending.code, pending.api ?: apiUrl) }, enabled = !busy)
                TextLink("Not this server", onClick = { foreign = null; apiUrl = AppSettings.DEFAULT_API_URL; showServer = false })
            }
        } else {
            FilledButton(
                if (busy) "Pairing…" else "Pair",
                onClick = ::pair,
                enabled = code.replace("-", "").length >= 8 && !busy,
                modifier = Modifier.fillMaxWidth(),
            )
        }
    }
}
