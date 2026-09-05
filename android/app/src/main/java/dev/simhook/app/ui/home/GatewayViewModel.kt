package dev.simhook.app.ui.home

import android.content.Context
import androidx.lifecycle.ViewModel
import androidx.lifecycle.viewModelScope
import dev.simhook.app.AppContainer
import dev.simhook.app.api.ApiException
import dev.simhook.app.api.DevicePatch
import dev.simhook.app.core.AppSettings
import dev.simhook.app.gateway.GatewayService
import dev.simhook.app.update.UpdateChecker
import dev.simhook.app.update.UpdateInstaller
import dev.simhook.app.work.HeartbeatScheduler
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.SharingStarted
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.stateIn
import kotlinx.coroutines.launch

/** State and actions shared by the dashboard and settings tabs. */
class GatewayViewModel(private val container: AppContainer, private val appContext: Context) : ViewModel() {
    val settings: StateFlow<AppSettings> = container.settings.flow
        .stateIn(viewModelScope, SharingStarted.WhileSubscribed(5_000), AppSettings())

    val queued: StateFlow<Int> = container.outbox.inFlightCountFlow()
        .stateIn(viewModelScope, SharingStarted.WhileSubscribed(5_000), 0)

    private val _busy = MutableStateFlow(false)
    val busy: StateFlow<Boolean> = _busy

    private val _notice = MutableStateFlow<String?>(null)
    val notice: StateFlow<String?> = _notice

    fun consumeNotice() {
        _notice.value = null
    }

    fun checkInNow() {
        HeartbeatScheduler.runNow(appContext)
        _notice.value = "Checking in with the server"
    }

    fun refresh() = run { viewModelScope.launch { guard { container.applyServerDevice(container.api.self()) } } }

    fun setGatewayEnabled(enabled: Boolean) = patch(DevicePatch(enabled = enabled))
    fun setReceiveEnabled(enabled: Boolean) = patch(DevicePatch(receiveEnabled = enabled))
    fun setSendDelay(seconds: Int) = patch(DevicePatch(sendDelaySeconds = seconds.coerceIn(0, 3600)))
    fun rename(name: String) = patch(DevicePatch(name = name.trim().take(64)))

    fun setPreferredSim(subscriptionId: Int?) = patch(
        if (subscriptionId == null) DevicePatch(clearPreferredSim = true) else DevicePatch(preferredSimSubscriptionId = subscriptionId),
    )

    fun setKeepAlive(enabled: Boolean) {
        viewModelScope.launch {
            container.settings.setKeepAlive(enabled)
            if (enabled) GatewayService.start(appContext) else if (queued.value == 0) GatewayService.stop(appContext)
        }
    }

    fun unpair() {
        viewModelScope.launch { container.unpair() }
    }

    /** Fetches the update manifest and reports the outcome. */
    fun checkForUpdates() {
        viewModelScope.launch {
            _notice.value = UpdateChecker.check(appContext, force = true).fold(
                onSuccess = { it?.let { u -> "Version ${u.versionName} is available." } ?: "You have the latest version." },
                onFailure = { "Could not check for updates." },
            )
        }
    }

    /** Reuses a recent check; only fetches when the last one is hours old. */
    fun checkForUpdatesQuietly() {
        viewModelScope.launch { UpdateChecker.check(appContext, force = false) }
    }

    fun installUpdate() {
        viewModelScope.launch {
            val update = container.settings.current().update ?: return@launch
            if (UpdateInstaller.install(appContext)) return@launch
            _notice.value = if (UpdateInstaller.start(appContext, update)) {
                "Downloading simhook ${update.versionName}"
            } else {
                "The update is already downloading."
            }
        }
    }

    private fun patch(p: DevicePatch) {
        viewModelScope.launch { guard { container.updateDevice(p) } }
    }

    private suspend fun guard(block: suspend () -> Unit) {
        _busy.value = true
        try {
            block()
        } catch (e: ApiException) {
            if (e.isAuthFailure) container.handleLostPairing()
            _notice.value = e.message
        } catch (e: Exception) {
            _notice.value = "Could not reach the server."
        } finally {
            _busy.value = false
        }
    }
}
