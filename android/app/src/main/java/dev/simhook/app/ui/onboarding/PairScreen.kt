package dev.simhook.app.ui.onboarding

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.safeDrawingPadding
import androidx.compose.foundation.layout.size
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.text.KeyboardOptions
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.Button
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedButton
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
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
import androidx.compose.ui.text.input.KeyboardCapitalization
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import com.google.mlkit.vision.barcode.common.Barcode
import com.google.mlkit.vision.codescanner.GmsBarcodeScannerOptions
import com.google.mlkit.vision.codescanner.GmsBarcodeScanning
import dev.simhook.app.AppContainer
import dev.simhook.app.api.ApiException
import dev.simhook.app.core.AppSettings
import dev.simhook.app.push.Push
import dev.simhook.app.ui.PairLink
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

    fun pair(withCode: String = code, withApi: String = apiUrl) {
        if (busy) return
        scope.launch {
            busy = true
            error = null
            try {
                val token = Push.token(context)
                container.pair(withCode.trim(), withApi.trim(), token)
            } catch (e: ApiException) {
                error = e.message
            } catch (e: Exception) {
                error = "Could not reach the server. Check the address and your connection. (${e.javaClass.simpleName}: ${e.message})"
            } finally {
                busy = false
            }
        }
    }

    // A link or scanned code carries everything needed, so it pairs at once.
    fun applyLink(parsed: PairLink) {
        code = parsed.code
        parsed.api?.let { apiUrl = it; showServer = it != AppSettings.DEFAULT_API_URL }
        error = null
        pair(parsed.code, parsed.api ?: apiUrl)
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
            .padding(24.dp),
        verticalArrangement = Arrangement.spacedBy(16.dp),
    ) {
        Spacer(Modifier.height(24.dp))
        Text("Pair this phone", style = MaterialTheme.typography.headlineMedium)
        Text(
            "In the dashboard, open Devices and create a pairing code. Scan it, or type it below. The code works once and expires in ten minutes.",
            style = MaterialTheme.typography.bodyLarge,
            color = MaterialTheme.colorScheme.onSurfaceVariant,
        )
        OutlinedTextField(
            value = code,
            onValueChange = { code = it.uppercase().take(9) },
            label = { Text("Pairing code") },
            placeholder = { Text("ABCD-EFGH") },
            singleLine = true,
            keyboardOptions = KeyboardOptions(capitalization = KeyboardCapitalization.Characters, keyboardType = KeyboardType.Ascii),
            modifier = Modifier.fillMaxWidth(),
        )
        OutlinedButton(onClick = ::scan, modifier = Modifier.fillMaxWidth(), enabled = !busy) { Text("Scan QR code") }
        if (showServer) {
            OutlinedTextField(
                value = apiUrl,
                onValueChange = { apiUrl = it },
                label = { Text("Server address") },
                singleLine = true,
                keyboardOptions = KeyboardOptions(keyboardType = KeyboardType.Uri),
                modifier = Modifier.fillMaxWidth(),
            )
        } else {
            TextButton(onClick = { showServer = true }) { Text("Use a different server") }
        }
        error?.let { Text(it, color = MaterialTheme.colorScheme.error, style = MaterialTheme.typography.bodyMedium) }
        Button(
            onClick = ::pair,
            enabled = code.replace("-", "").length >= 8 && !busy,
            modifier = Modifier.fillMaxWidth(),
        ) {
            if (busy) CircularProgressIndicator(modifier = Modifier.size(18.dp), strokeWidth = 2.dp) else Text("Pair")
        }
    }
}
