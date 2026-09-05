package dev.simhook.app.ui

import android.content.Intent
import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.compose.runtime.mutableStateOf
import dev.simhook.app.SimhookApp
import dev.simhook.app.core.AppVisibility
import dev.simhook.app.core.PairLink
import dev.simhook.app.ui.theme.SimhookTheme

class MainActivity : ComponentActivity() {
    private val pendingLink = mutableStateOf<PairLink?>(null)

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        enableEdgeToEdge()
        pendingLink.value = PairLink.parse(intent?.dataString)
        val container = SimhookApp.get(this).container
        setContent {
            SimhookTheme {
                AppRoot(container = container, link = pendingLink.value, onLinkConsumed = { pendingLink.value = null })
            }
        }
    }

    override fun onNewIntent(intent: Intent) {
        super.onNewIntent(intent)
        PairLink.parse(intent.dataString)?.let { pendingLink.value = it }
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
