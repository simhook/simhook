package dev.simhook.app.core

import android.content.Context
import androidx.datastore.core.DataStore
import androidx.datastore.preferences.core.Preferences
import androidx.datastore.preferences.core.booleanPreferencesKey
import androidx.datastore.preferences.core.edit
import androidx.datastore.preferences.core.intPreferencesKey
import androidx.datastore.preferences.core.longPreferencesKey
import androidx.datastore.preferences.core.stringPreferencesKey
import androidx.datastore.preferences.preferencesDataStore
import kotlinx.coroutines.flow.Flow
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.flow.map

private val Context.dataStore: DataStore<Preferences> by preferencesDataStore(name = "settings")

/** Everything the app remembers between launches. The device token is not here; see [SecureStore]. */
data class AppSettings(
    val apiBaseUrl: String = DEFAULT_API_URL,
    val deviceId: String? = null,
    val deviceName: String = "",
    val gatewayEnabled: Boolean = true,
    val receiveEnabled: Boolean = false,
    val sendDelaySeconds: Int = 5,
    val heartbeatIntervalMinutes: Int = 20,
    val preferredSimSubscriptionId: Int? = null,
    val keepAliveNotification: Boolean = false,
    val lastHeartbeatAt: Long = 0L,
    val pushToken: String? = null,
    val sentCount: Long = 0L,
    val receivedCount: Long = 0L,
    val online: Boolean = false,
) {
    val isPaired: Boolean get() = deviceId != null

    companion object {
        const val DEFAULT_API_URL = "https://api.simhook.dev"
    }
}

class SettingsStore(private val context: Context) {
    private object Keys {
        val apiBaseUrl = stringPreferencesKey("api_base_url")
        val deviceId = stringPreferencesKey("device_id")
        val deviceName = stringPreferencesKey("device_name")
        val gatewayEnabled = booleanPreferencesKey("gateway_enabled")
        val receiveEnabled = booleanPreferencesKey("receive_enabled")
        val sendDelaySeconds = intPreferencesKey("send_delay_seconds")
        val heartbeatIntervalMinutes = intPreferencesKey("heartbeat_interval_minutes")
        val preferredSim = intPreferencesKey("preferred_sim")
        val keepAlive = booleanPreferencesKey("keep_alive_notification")
        val lastHeartbeatAt = longPreferencesKey("last_heartbeat_at")
        val pushToken = stringPreferencesKey("push_token")
        val sentCount = longPreferencesKey("sent_count")
        val receivedCount = longPreferencesKey("received_count")
        val online = booleanPreferencesKey("online")
    }

    val flow: Flow<AppSettings> = context.dataStore.data.map { p ->
        AppSettings(
            apiBaseUrl = p[Keys.apiBaseUrl] ?: AppSettings.DEFAULT_API_URL,
            deviceId = p[Keys.deviceId],
            deviceName = p[Keys.deviceName] ?: "",
            gatewayEnabled = p[Keys.gatewayEnabled] ?: true,
            receiveEnabled = p[Keys.receiveEnabled] ?: false,
            sendDelaySeconds = p[Keys.sendDelaySeconds] ?: 5,
            heartbeatIntervalMinutes = p[Keys.heartbeatIntervalMinutes] ?: 20,
            preferredSimSubscriptionId = p[Keys.preferredSim]?.takeIf { it >= 0 },
            keepAliveNotification = p[Keys.keepAlive] ?: false,
            lastHeartbeatAt = p[Keys.lastHeartbeatAt] ?: 0L,
            pushToken = p[Keys.pushToken],
            sentCount = p[Keys.sentCount] ?: 0L,
            receivedCount = p[Keys.receivedCount] ?: 0L,
            online = p[Keys.online] ?: false,
        )
    }

    suspend fun current(): AppSettings = flow.first()

    suspend fun setApiBaseUrl(url: String) = edit { it[Keys.apiBaseUrl] = url.trimEnd('/') }

    suspend fun setPaired(deviceId: String, name: String) = edit {
        it[Keys.deviceId] = deviceId
        it[Keys.deviceName] = name
    }

    suspend fun setGatewayEnabled(v: Boolean) = edit { it[Keys.gatewayEnabled] = v }
    suspend fun setReceiveEnabled(v: Boolean) = edit { it[Keys.receiveEnabled] = v }
    suspend fun setKeepAlive(v: Boolean) = edit { it[Keys.keepAlive] = v }
    suspend fun setPushToken(token: String) = edit { it[Keys.pushToken] = token }
    suspend fun setLastHeartbeat(at: Long) = edit { it[Keys.lastHeartbeatAt] = at }

    /** Apply the settings the server considers canonical. */
    suspend fun applyFromServer(
        name: String,
        enabled: Boolean,
        receiveEnabled: Boolean,
        sendDelaySeconds: Int,
        heartbeatIntervalMinutes: Int,
        preferredSim: Int?,
        sentCount: Long,
        receivedCount: Long,
        online: Boolean,
    ) = edit {
        it[Keys.deviceName] = name
        it[Keys.gatewayEnabled] = enabled
        it[Keys.receiveEnabled] = receiveEnabled
        it[Keys.sendDelaySeconds] = sendDelaySeconds
        it[Keys.heartbeatIntervalMinutes] = heartbeatIntervalMinutes
        it[Keys.preferredSim] = preferredSim ?: -1
        it[Keys.sentCount] = sentCount
        it[Keys.receivedCount] = receivedCount
        it[Keys.online] = online
    }

    /** Forget the pairing but keep the API URL so re-pairing is one step. */
    suspend fun clearPairing() = edit { p ->
        listOf(
            Keys.deviceId, Keys.deviceName, Keys.gatewayEnabled, Keys.receiveEnabled, Keys.sendDelaySeconds,
            Keys.heartbeatIntervalMinutes, Keys.preferredSim, Keys.keepAlive, Keys.lastHeartbeatAt,
            Keys.sentCount, Keys.receivedCount, Keys.online,
        ).forEach { p.remove(it) }
    }

    private suspend fun edit(block: (androidx.datastore.preferences.core.MutablePreferences) -> Unit) {
        context.dataStore.edit { block(it) }
    }
}
