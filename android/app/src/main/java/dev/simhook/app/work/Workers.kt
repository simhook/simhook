package dev.simhook.app.work

import android.content.Context
import androidx.work.BackoffPolicy
import androidx.work.Constraints
import androidx.work.CoroutineWorker
import androidx.work.Data
import androidx.work.ExistingPeriodicWorkPolicy
import androidx.work.ExistingWorkPolicy
import androidx.work.NetworkType
import androidx.work.OneTimeWorkRequestBuilder
import androidx.work.OutOfQuotaPolicy
import androidx.work.PeriodicWorkRequestBuilder
import androidx.work.WorkManager
import androidx.work.WorkerParameters
import androidx.work.workDataOf
import dev.simhook.app.BuildConfig
import dev.simhook.app.SimhookApp
import dev.simhook.app.api.ApiException
import dev.simhook.app.api.HeartbeatRequest
import dev.simhook.app.api.InboundReport
import dev.simhook.app.api.StatusReport
import dev.simhook.app.core.DeviceIdentity
import dev.simhook.app.core.Notifications
import dev.simhook.app.core.TelemetryCollector
import dev.simhook.app.push.Push
import dev.simhook.app.sms.SimInfo
import java.io.IOException
import java.time.Instant
import java.util.concurrent.TimeUnit

private val networkConstraints = Constraints.Builder().setRequiredNetworkType(NetworkType.CONNECTED).build()

private fun iso(millis: Long): String = Instant.ofEpochMilli(millis).toString()

/** Decides whether a failed API call is worth retrying. */
private fun retryOrFail(e: Exception, attempt: Int, maxAttempts: Int): androidx.work.ListenableWorker.Result = when {
    e is ApiException && e.status in 400..499 && e.status != 429 && e.status != 408 -> androidx.work.ListenableWorker.Result.failure()
    attempt >= maxAttempts -> androidx.work.ListenableWorker.Result.failure()
    else -> androidx.work.ListenableWorker.Result.retry()
}

// ---------------------------------------------------------------------------
// Heartbeat
// ---------------------------------------------------------------------------

class HeartbeatWorker(context: Context, params: WorkerParameters) : CoroutineWorker(context, params) {
    override suspend fun doWork(): Result {
        val container = SimhookApp.get(applicationContext).container
        val settings = container.settings.current()
        if (!settings.isPaired) return Result.success()
        val token = Push.token(applicationContext)
        val request = HeartbeatRequest(
            pushToken = token,
            appVersionName = BuildConfig.VERSION_NAME,
            appVersionCode = BuildConfig.VERSION_CODE,
            osVersion = DeviceIdentity.osVersion,
            osApiLevel = DeviceIdentity.osApiLevel,
            telemetry = TelemetryCollector.collect(applicationContext, settings.keepAliveNotification, container.outbox.inFlightCount()),
            sims = SimInfo.list(applicationContext),
        )
        return try {
            val device = container.api.heartbeat(request)
            container.applyServerDevice(device)
            container.settings.setLastHeartbeat(System.currentTimeMillis())
            if (token != null) container.settings.setPushToken(token)
            Result.success()
        } catch (e: ApiException) {
            if (e.isAuthFailure) {
                container.handleLostPairing()
                return Result.failure()
            }
            retryOrFail(e, runAttemptCount, 5)
        } catch (e: IOException) {
            retryOrFail(e, runAttemptCount, 5)
        }
    }
}

object HeartbeatScheduler {
    private const val PERIODIC = "heartbeat"
    private const val NOW = "heartbeat-now"

    /** Keeps a periodic check-in scheduled at the interval the server asked for. */
    fun ensure(context: Context, intervalMinutes: Int) {
        val minutes = intervalMinutes.coerceIn(15, 24 * 60).toLong()
        val request = PeriodicWorkRequestBuilder<HeartbeatWorker>(minutes, TimeUnit.MINUTES)
            .setConstraints(networkConstraints)
            .setBackoffCriteria(BackoffPolicy.EXPONENTIAL, 30, TimeUnit.SECONDS)
            .build()
        WorkManager.getInstance(context).enqueueUniquePeriodicWork(PERIODIC, ExistingPeriodicWorkPolicy.UPDATE, request)
    }

    fun runNow(context: Context) {
        val request = OneTimeWorkRequestBuilder<HeartbeatWorker>()
            .setConstraints(networkConstraints)
            .setExpedited(OutOfQuotaPolicy.RUN_AS_NON_EXPEDITED_WORK_REQUEST)
            .setBackoffCriteria(BackoffPolicy.EXPONENTIAL, 15, TimeUnit.SECONDS)
            .build()
        WorkManager.getInstance(context).enqueueUniqueWork(NOW, ExistingWorkPolicy.REPLACE, request)
    }

    fun cancel(context: Context) {
        WorkManager.getInstance(context).cancelUniqueWork(PERIODIC)
        WorkManager.getInstance(context).cancelUniqueWork(NOW)
    }
}

// ---------------------------------------------------------------------------
// Status reports
// ---------------------------------------------------------------------------

class StatusReportWorker(context: Context, params: WorkerParameters) : CoroutineWorker(context, params) {
    override suspend fun doWork(): Result {
        val id = inputData.getString(KEY_ID) ?: return Result.failure()
        val status = inputData.getString(KEY_STATUS) ?: return Result.failure()
        val at = inputData.getLong(KEY_AT, System.currentTimeMillis())
        val report = StatusReport(
            status = status,
            at = iso(at),
            errorCode = inputData.getString(KEY_CODE),
            errorMessage = inputData.getString(KEY_MESSAGE),
        )
        val container = SimhookApp.get(applicationContext).container
        return try {
            container.api.reportStatus(id, report)
            Result.success()
        } catch (e: ApiException) {
            if (e.isAuthFailure) container.handleLostPairing()
            retryOrFail(e, runAttemptCount, 8)
        } catch (e: IOException) {
            retryOrFail(e, runAttemptCount, 8)
        }
    }

    companion object {
        private const val KEY_ID = "id"
        private const val KEY_STATUS = "status"
        private const val KEY_AT = "at"
        private const val KEY_CODE = "code"
        private const val KEY_MESSAGE = "message"

        fun enqueue(context: Context, messageId: String, status: String, at: Long, code: String?, message: String?) {
            val data = workDataOf(KEY_ID to messageId, KEY_STATUS to status, KEY_AT to at, KEY_CODE to code, KEY_MESSAGE to message)
            val request = OneTimeWorkRequestBuilder<StatusReportWorker>()
                .setInputData(data)
                .setConstraints(networkConstraints)
                .setBackoffCriteria(BackoffPolicy.EXPONENTIAL, 10, TimeUnit.SECONDS)
                .build()
            WorkManager.getInstance(context).enqueueUniqueWork("status-$messageId-$status", ExistingWorkPolicy.KEEP, request)
        }
    }
}

// ---------------------------------------------------------------------------
// Inbound uploads
// ---------------------------------------------------------------------------

class InboundUploadWorker(context: Context, params: WorkerParameters) : CoroutineWorker(context, params) {
    override suspend fun doWork(): Result {
        val sender = inputData.getString(KEY_SENDER) ?: return Result.failure()
        val body = inputData.getString(KEY_BODY) ?: return Result.failure()
        val receivedAt = inputData.getLong(KEY_RECEIVED_AT, System.currentTimeMillis())
        val fingerprint = inputData.getString(KEY_FINGERPRINT) ?: return Result.failure()
        val sim = inputData.getInt(KEY_SIM, -1).takeIf { it >= 0 }
        val container = SimhookApp.get(applicationContext).container
        return try {
            container.api.reportInbound(InboundReport(sender, body, iso(receivedAt), fingerprint, sim))
            Result.success()
        } catch (e: ApiException) {
            if (e.isAuthFailure) container.handleLostPairing()
            retryOrFail(e, runAttemptCount, 10)
        } catch (e: IOException) {
            retryOrFail(e, runAttemptCount, 10)
        }
    }

    companion object {
        private const val KEY_SENDER = "sender"
        private const val KEY_BODY = "body"
        private const val KEY_RECEIVED_AT = "received_at"
        private const val KEY_FINGERPRINT = "fingerprint"
        private const val KEY_SIM = "sim"

        fun enqueue(context: Context, sender: String, body: String, receivedAt: Long, fingerprint: String, sim: Int?) {
            val data = Data.Builder()
                .putString(KEY_SENDER, sender)
                .putString(KEY_BODY, body)
                .putLong(KEY_RECEIVED_AT, receivedAt)
                .putString(KEY_FINGERPRINT, fingerprint)
                .putInt(KEY_SIM, sim ?: -1)
                .build()
            val request = OneTimeWorkRequestBuilder<InboundUploadWorker>()
                .setInputData(data)
                .setConstraints(networkConstraints)
                .setExpedited(OutOfQuotaPolicy.RUN_AS_NON_EXPEDITED_WORK_REQUEST)
                .setBackoffCriteria(BackoffPolicy.EXPONENTIAL, 10, TimeUnit.SECONDS)
                .build()
            WorkManager.getInstance(context).enqueueUniqueWork("inbound-$fingerprint", ExistingWorkPolicy.KEEP, request)
        }
    }
}

// ---------------------------------------------------------------------------
// Push token refresh
// ---------------------------------------------------------------------------

class PushTokenWorker(context: Context, params: WorkerParameters) : CoroutineWorker(context, params) {
    override suspend fun doWork(): Result {
        val token = inputData.getString(KEY_TOKEN) ?: return Result.failure()
        val container = SimhookApp.get(applicationContext).container
        if (!container.settings.current().isPaired) return Result.success()
        return try {
            container.api.pushToken(token)
            Result.success()
        } catch (e: ApiException) {
            if (e.isAuthFailure) container.handleLostPairing()
            retryOrFail(e, runAttemptCount, 5)
        } catch (e: IOException) {
            retryOrFail(e, runAttemptCount, 5)
        }
    }

    companion object {
        private const val KEY_TOKEN = "token"

        fun enqueue(context: Context, token: String) {
            val request = OneTimeWorkRequestBuilder<PushTokenWorker>()
                .setInputData(workDataOf(KEY_TOKEN to token))
                .setConstraints(networkConstraints)
                .setBackoffCriteria(BackoffPolicy.EXPONENTIAL, 15, TimeUnit.SECONDS)
                .build()
            WorkManager.getInstance(context).enqueueUniqueWork("push-token", ExistingWorkPolicy.REPLACE, request)
        }
    }
}

/** Surfaces a lost pairing to the user; used by the container. */
internal fun notifyPairingLost(context: Context) {
    Notifications.alert(
        context, Notifications.ID_ALERT_PAIRING,
        "This phone was unpaired",
        "The server no longer accepts this phone. Open the app to pair it again.",
    )
}
