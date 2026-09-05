package dev.simhook.app.ui

import androidx.compose.runtime.Composable
import androidx.compose.runtime.DisposableEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.platform.LocalContext
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.LifecycleEventObserver
import androidx.lifecycle.compose.LocalLifecycleOwner
import dev.simhook.app.api.SimDescriptor
import dev.simhook.app.sms.SimInfo

/** The SIMs in the phone, read again whenever the screen comes back, so a granted permission or a swapped SIM shows at once. */
@Composable
fun rememberSims(): List<SimDescriptor> {
    val context = LocalContext.current
    val owner = LocalLifecycleOwner.current
    var sims by remember { mutableStateOf(SimInfo.list(context)) }
    DisposableEffect(owner) {
        val observer = LifecycleEventObserver { _, event ->
            if (event == Lifecycle.Event.ON_RESUME) sims = SimInfo.list(context)
        }
        owner.lifecycle.addObserver(observer)
        onDispose { owner.lifecycle.removeObserver(observer) }
    }
    return sims
}
