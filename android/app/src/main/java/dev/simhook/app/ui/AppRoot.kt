package dev.simhook.app.ui

import androidx.compose.foundation.layout.Box
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.material3.CircularProgressIndicator
import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.saveable.rememberSaveable
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.unit.dp
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import androidx.lifecycle.compose.LocalLifecycleOwner
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import dev.simhook.app.AppContainer
import dev.simhook.app.core.PairLink
import dev.simhook.app.ui.home.HomeScreen
import dev.simhook.app.ui.onboarding.PairScreen
import dev.simhook.app.ui.onboarding.PermissionsScreen
import dev.simhook.app.ui.theme.Tokens

/** Routes between pairing, permissions, and the main screens based on state alone. */
@Composable
fun AppRoot(container: AppContainer, link: PairLink?, onLinkConsumed: () -> Unit) {
    val settings by container.settings.flow.collectAsStateWithLifecycle(initialValue = null)
    val context = LocalContext.current
    val lifecycleOwner = LocalLifecycleOwner.current
    var coreGranted by remember { mutableStateOf(Permissions.coreGranted(context)) }
    var permissionsSkipped by rememberSaveable { mutableStateOf(false) }

    DisposableEffect(lifecycleOwner) {
        val observer = LifecycleEventObserver { _, event ->
            if (event == Lifecycle.Event.ON_RESUME) coreGranted = Permissions.coreGranted(context)
        }
        lifecycleOwner.lifecycle.addObserver(observer)
        onDispose { lifecycleOwner.lifecycle.removeObserver(observer) }
    }

    val current = settings
    when {
        current == null -> Box(Modifier.fillMaxSize(), contentAlignment = Alignment.Center) {
            CircularProgressIndicator(color = Tokens.Fg, strokeWidth = 1.5.dp)
        }
        !current.isPaired -> PairScreen(container = container, link = link, onLinkConsumed = onLinkConsumed)
        !coreGranted && !permissionsSkipped -> PermissionsScreen(onDone = {
            coreGranted = Permissions.coreGranted(context)
            permissionsSkipped = true
        })
        else -> HomeScreen(container = container, permissionsOk = coreGranted)
    }
}
