package dev.simhook.app.api

import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable

/** The device record as the server holds it. Fields the app does not use are ignored. */
@Serializable
data class Device(
    val id: String,
    val name: String,
    val enabled: Boolean = true,
    @SerialName("is_default") val isDefault: Boolean = false,
    @SerialName("receive_enabled") val receiveEnabled: Boolean = true,
    @SerialName("send_delay_seconds") val sendDelaySeconds: Int = 5,
    @SerialName("heartbeat_interval_minutes") val heartbeatIntervalMinutes: Int = 20,
    @SerialName("preferred_sim_subscription_id") val preferredSimSubscriptionId: Int? = null,
    @SerialName("last_heartbeat_at") val lastHeartbeatAt: String? = null,
    val online: Boolean = false,
    @SerialName("sent_count") val sentCount: Long = 0,
    @SerialName("received_count") val receivedCount: Long = 0,
    @SerialName("push_token_invalidated_at") val pushTokenInvalidatedAt: String? = null,
)

@Serializable
data class PairRequest(
    val code: String,
    @SerialName("hardware_key") val hardwareKey: String,
    val name: String? = null,
    val manufacturer: String? = null,
    val brand: String? = null,
    val model: String? = null,
    @SerialName("build_id") val buildId: String? = null,
    @SerialName("os_version") val osVersion: String? = null,
    @SerialName("os_api_level") val osApiLevel: Int? = null,
    @SerialName("app_version_name") val appVersionName: String? = null,
    @SerialName("app_version_code") val appVersionCode: Int? = null,
    @SerialName("push_token") val pushToken: String? = null,
)

@Serializable
data class PairResponse(
    val device: Device,
    @SerialName("device_token") val deviceToken: String,
)

@Serializable
data class DeviceEnvelope(val device: Device)

@Serializable
data class DevicePatch(
    val name: String? = null,
    val enabled: Boolean? = null,
    @SerialName("receive_enabled") val receiveEnabled: Boolean? = null,
    @SerialName("send_delay_seconds") val sendDelaySeconds: Int? = null,
    @SerialName("preferred_sim_subscription_id") val preferredSimSubscriptionId: Int? = null,
    @SerialName("clear_preferred_sim") val clearPreferredSim: Boolean? = null,
)

@Serializable
data class SimDescriptor(
    @SerialName("subscription_id") val subscriptionId: Int,
    val slot: Int,
    val carrier: String? = null,
    @SerialName("display_name") val displayName: String? = null,
    val country: String? = null,
)

@Serializable
data class Telemetry(
    @SerialName("battery_percent") val batteryPercent: Int? = null,
    val charging: Boolean? = null,
    val network: String? = null,
    @SerialName("uptime_ms") val uptimeMs: Long? = null,
    val timezone: String? = null,
    val locale: String? = null,
    @SerialName("storage_free_bytes") val storageFreeBytes: Long? = null,
    @SerialName("keep_alive") val keepAlive: Boolean? = null,
    @SerialName("outbox_pending") val outboxPending: Int? = null,
)

@Serializable
data class HeartbeatRequest(
    @SerialName("push_token") val pushToken: String? = null,
    @SerialName("app_version_name") val appVersionName: String? = null,
    @SerialName("app_version_code") val appVersionCode: Int? = null,
    @SerialName("os_version") val osVersion: String? = null,
    @SerialName("os_api_level") val osApiLevel: Int? = null,
    val telemetry: Telemetry? = null,
    val sims: List<SimDescriptor>? = null,
)

@Serializable
data class PushTokenRequest(val token: String)

@Serializable
data class OutboxItem(
    val id: String,
    @SerialName("batch_id") val batchId: String? = null,
    val to: String,
    val body: String,
    @SerialName("sim_subscription_id") val simSubscriptionId: Int? = null,
)

@Serializable
data class OutboxPage(val data: List<OutboxItem> = emptyList())

@Serializable
data class StatusReport(
    val status: String,
    val at: String,
    @SerialName("error_code") val errorCode: String? = null,
    @SerialName("error_message") val errorMessage: String? = null,
)

@Serializable
data class InboundReport(
    val sender: String,
    val body: String,
    @SerialName("received_at") val receivedAt: String,
    val fingerprint: String,
    @SerialName("sim_subscription_id") val simSubscriptionId: Int? = null,
)

@Serializable
data class Message(
    val id: String,
    val direction: String,
    val status: String,
    val body: String,
    val recipient: String? = null,
    val sender: String? = null,
    @SerialName("error_code") val errorCode: String? = null,
    @SerialName("error_message") val errorMessage: String? = null,
    @SerialName("created_at") val createdAt: String,
    @SerialName("received_at") val receivedAt: String? = null,
    @SerialName("delivered_at") val deliveredAt: String? = null,
)

@Serializable
data class MessagePage(
    val data: List<Message>,
    @SerialName("next_cursor") val nextCursor: String? = null,
)

@Serializable
data class ApiErrorBody(
    val status: Int = 0,
    val code: String = "error",
    val message: String = "",
    val errors: List<FieldError> = emptyList(),
)

@Serializable
data class FieldError(val field: String? = null, val message: String)
