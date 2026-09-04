package dev.simhook.app.api

import dev.simhook.app.core.AppJson
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import kotlinx.serialization.encodeToString
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import java.io.IOException
import java.util.concurrent.TimeUnit

/** Raised for any non-2xx response. [code] is the server's stable error code. */
class ApiException(val status: Int, val code: String, message: String) : IOException(message) {
    val isAuthFailure: Boolean get() = status == 401
}

/**
 * The phone's view of the API. Every call is a suspend function that either
 * returns a decoded body or throws [ApiException] / [IOException].
 */
class ApiClient(
    private val baseUrl: suspend () -> String,
    private val deviceToken: suspend () -> String?,
    private val userAgent: String,
) {
    private val http = OkHttpClient.Builder()
        .connectTimeout(15, TimeUnit.SECONDS)
        .readTimeout(30, TimeUnit.SECONDS)
        .writeTimeout(30, TimeUnit.SECONDS)
        .retryOnConnectionFailure(true)
        .build()

    suspend fun pair(request: PairRequest): PairResponse =
        post("/v1/device/pair", AppJson.encodeToString(request), auth = false)

    suspend fun self(): Device = get<DeviceEnvelope>("/v1/device").device

    suspend fun update(patch: DevicePatch): Device =
        patch<DeviceEnvelope>("/v1/device", AppJson.encodeToString(patch)).device

    suspend fun heartbeat(request: HeartbeatRequest): Device =
        post<DeviceEnvelope>("/v1/device/heartbeat", AppJson.encodeToString(request)).device

    suspend fun pushToken(token: String) {
        post<Unit>("/v1/device/push-token", AppJson.encodeToString(PushTokenRequest(token)))
    }

    suspend fun reportStatus(messageId: String, report: StatusReport): Message =
        post<MessageEnvelope>("/v1/device/messages/$messageId/status", AppJson.encodeToString(report)).message

    suspend fun reportInbound(report: InboundReport): Message =
        post<InboundEnvelope>("/v1/device/inbound", AppJson.encodeToString(report)).message

    suspend fun messages(direction: String?, cursor: String?, limit: Int = 50): MessagePage {
        val query = buildList {
            direction?.let { add("direction=$it") }
            cursor?.let { add("cursor=$it") }
            add("limit=$limit")
        }.joinToString("&")
        return get("/v1/device/messages?$query")
    }

    // ---------------------------------------------------------------------

    @kotlinx.serialization.Serializable
    private data class MessageEnvelope(val message: Message)

    @kotlinx.serialization.Serializable
    private data class InboundEnvelope(val message: Message, val inserted: Boolean = true)

    private suspend inline fun <reified T> get(path: String): T = execute(path, "GET", null, auth = true)

    private suspend inline fun <reified T> post(path: String, body: String, auth: Boolean = true): T =
        execute(path, "POST", body, auth)

    private suspend inline fun <reified T> patch(path: String, body: String): T = execute(path, "PATCH", body, auth = true)

    private suspend inline fun <reified T> execute(path: String, method: String, body: String?, auth: Boolean): T = withContext(Dispatchers.IO) {
        val builder = Request.Builder()
            .url(baseUrl().trimEnd('/') + path)
            .method(method, body?.toRequestBody(JSON))
            .header("User-Agent", userAgent)
            .header("Accept", "application/json")
        if (auth) {
            val token = deviceToken() ?: throw ApiException(401, "not_paired", "This phone is not paired.")
            builder.header("Authorization", "Bearer $token")
        }
        http.newCall(builder.build()).execute().use { response ->
            val text = response.body.string()
            if (!response.isSuccessful) {
                val parsed = runCatching { AppJson.decodeFromString(ApiErrorBody.serializer(), text) }.getOrNull()
                throw ApiException(
                    response.code,
                    parsed?.code ?: "http_${response.code}",
                    parsed?.message?.takeIf { it.isNotBlank() } ?: "Request failed with HTTP ${response.code}",
                )
            }
            if (T::class == Unit::class) return@use Unit as T
            AppJson.decodeFromString(text)
        }
    }

    companion object {
        private val JSON = "application/json; charset=utf-8".toMediaType()
    }
}
