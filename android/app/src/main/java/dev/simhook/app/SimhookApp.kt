package dev.simhook.app

import android.app.Application
import android.content.Context
import dev.simhook.app.api.ApiClient
import dev.simhook.app.api.Device
import dev.simhook.app.api.DevicePatch
import dev.simhook.app.api.PairRequest
import dev.simhook.app.core.DeviceIdentity
import dev.simhook.app.core.Notifications
import dev.simhook.app.core.SecureStore
import dev.simhook.app.core.SettingsStore
import dev.simhook.app.gateway.GatewayService
import dev.simhook.app.outbox.AppDatabase
import dev.simhook.app.outbox.OutboxDao
import dev.simhook.app.update.UpdateScheduler
import dev.simhook.app.work.HeartbeatScheduler
import dev.simhook.app.work.notifyPairingLost
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.launch

class SimhookApp : Application() {
    lateinit var container: AppContainer
        private set

    override fun onCreate() {
        super.onCreate()
        container = AppContainer(this)
        Notifications.createChannels(this)
        UpdateScheduler.ensure(this)
        container.scope.launch {
            val settings = container.settings.current()
            if (settings.isPaired) {
                HeartbeatScheduler.ensure(this@SimhookApp, settings.heartbeatIntervalMinutes)
                if (container.outbox.inFlightCount() > 0) GatewayService.startOrDrain(this@SimhookApp)
            }
        }
    }

    companion object {
        fun get(context: Context): SimhookApp = context.applicationContext as SimhookApp
    }
}

/** Plain dependency container. One instance per process. */
class AppContainer(private val context: Context) {
    val scope = CoroutineScope(SupervisorJob() + Dispatchers.Default)
    val settings = SettingsStore(context)
    val secure = SecureStore(context)
    val outbox: OutboxDao = AppDatabase.get(context).outbox()
    val api = ApiClient(
        baseUrl = { settings.current().apiBaseUrl },
        deviceToken = { secure.get(SecureStore.DEVICE_TOKEN) },
        userAgent = "simhook-android/${BuildConfig.VERSION_NAME}",
    )

    /** Exchanges a pairing code for a device record and token. */
    suspend fun pair(code: String, apiBaseUrl: String, pushToken: String?): Device {
        // Whatever an earlier pairing left behind is not this account's to send.
        outbox.clear()
        settings.setApiBaseUrl(apiBaseUrl)
        val current = settings.current()
        val response = api.pair(
            PairRequest(
                code = code,
                hardwareKey = DeviceIdentity.hardwareKey(context),
                name = DeviceIdentity.defaultName(),
                manufacturer = DeviceIdentity.manufacturer,
                brand = DeviceIdentity.brand,
                model = DeviceIdentity.model,
                buildId = DeviceIdentity.buildId,
                osVersion = DeviceIdentity.osVersion,
                osApiLevel = DeviceIdentity.osApiLevel,
                appVersionName = BuildConfig.VERSION_NAME,
                appVersionCode = BuildConfig.VERSION_CODE,
                pushToken = pushToken ?: current.pushToken,
            ),
        )
        secure.put(SecureStore.DEVICE_TOKEN, response.deviceToken)
        settings.setPaired(response.device.id, response.device.name)
        applyServerDevice(response.device)
        HeartbeatScheduler.ensure(context, response.device.heartbeatIntervalMinutes)
        HeartbeatScheduler.runNow(context)
        return response.device
    }

    /** Server values win; the phone mirrors them. */
    suspend fun applyServerDevice(device: Device) {
        settings.applyFromServer(
            name = device.name,
            enabled = device.enabled,
            receiveEnabled = device.receiveEnabled,
            sendDelaySeconds = device.sendDelaySeconds,
            heartbeatIntervalMinutes = device.heartbeatIntervalMinutes,
            preferredSim = device.preferredSimSubscriptionId,
            sentCount = device.sentCount,
            receivedCount = device.receivedCount,
            online = device.online,
        )
        HeartbeatScheduler.ensure(context, device.heartbeatIntervalMinutes)
    }

    /** A setting changed in the app: tell the server, then mirror its answer. */
    suspend fun updateDevice(patch: DevicePatch): Device {
        val device = api.update(patch)
        applyServerDevice(device)
        return device
    }

    /** Unpair on the server when reachable, then forget the pairing locally either way. */
    suspend fun unpair() {
        runCatching { api.unpair() }
        unpairLocally()
    }

    /** Forget this pairing locally. Used when the server already revoked the phone. */
    suspend fun unpairLocally() {
        HeartbeatScheduler.cancel(context)
        GatewayService.stop(context)
        secure.remove(SecureStore.DEVICE_TOKEN)
        outbox.clear()
        settings.clearPairing()
    }

    /** Called when the server rejects the device token. */
    suspend fun handleLostPairing() {
        if (!settings.current().isPaired) return
        unpairLocally()
        notifyPairingLost(context)
    }
}
