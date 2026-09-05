package dev.simhook.app.ui

import android.content.Intent
import android.net.Uri
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.compose.runtime.mutableStateOf
import dev.simhook.app.SimhookApp
import dev.simhook.app.core.AppVisibility
import dev.simhook.app.ui.theme.SimhookTheme

/** A pairing deep link: simhook://pair?code=XXXX-XXXX&api=https://api.example.com */
data class PairLink(val code: String, val api: String?) {
    companion object {
        fun parse(intent: Intent?): PairLink? = parse(intent?.data)

        fun parse(raw: String): PairLink? = runCatching { parse(Uri.parse(raw.trim())) }.getOrNull()

        private fun parse(uri: Uri?): PairLink? {
            if (uri == null || uri.scheme != "simhook" || uri.host != "pair") return null
            val code = uri.getQueryParameter("code")?.trim()?.takeIf { it.isNotEmpty() } ?: return null
            val api = uri.getQueryParameter("api")?.trim()?.takeIf { it.startsWith("http") }
            return PairLink(code, api)
        }
    }
}

class MainActivity : ComponentActivity() {
    private val pendingLink = mutableStateOf<PairLink?>(null)

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        pendingLink.value = PairLink.parse(intent)
        val container = SimhookApp.get(this).container
        setContent {
            SimhookTheme {
                AppRoot(container = container, link = pendingLink.value, onLinkConsumed = { pendingLink.value = null })
            }
        }
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        PairLink.parse(intent)?.let { pendingLink.value = it }
    }

    override fun onResume() {
        super.onResume()
        AppVisibility.visible = true
    }

    override fun onPause() {
        AppVisibility.visible = false
        super.onPause()
    }
}
