package dev.simhook.app.work

import android.content.Context
import android.content.pm.ServiceInfo
import android.os.Build
import android.util.Log
import androidx.work.BackoffPolicy
import androidx.work.Constraints
import androidx.work.CoroutineWorker
import androidx.work.ExistingPeriodicWorkPolicy
import androidx.work.ExistingWorkPolicy
import androidx.work.ForegroundInfo
import androidx.work.NetworkType
import androidx.work.OneTimeWorkRequestBuilder
import androidx.work.OutOfQuotaPolicy
import androidx.work.PeriodicWorkRequestBuilder
import androidx.work.WorkManager
import androidx.work.WorkerParameters
import androidx.work.workDataOf
import dev.simhook.app.AppContainer
import dev.simhook.app.BuildConfig
import dev.simhook.app.SimhookApp
import dev.simhook.app.api.ApiException
import dev.simhook.app.api.HeartbeatRequest
import dev.simhook.app.api.InboundReport
import dev.simhook.app.api.StatusReport
import dev.simhook.app.core.DeviceIdentity
import dev.simhook.app.core.Notifications
import dev.simhook.app.core.TelemetryCollector
import dev.simhook.app.gateway.GatewayService
import dev.simhook.app.outbox.OutboxDrainer
import dev.simhook.app.outbox.OutboxMessage
import dev.simhook.app.outbox.PendingReport
import dev.simhook.app.push.Push
import dev.simhook.app.sms.SimInfo
import java.io.IOException
import java.time.Instant
import java.util.concurrent.TimeUnit

private const val TAG = "Workers"

private val networkConstraints = Constraints.Builder().setRequiredNetworkType(NetworkType.CONNECTED).build()

private fun iso(millis: Long): String = Instant.ofEpochMilli(millis).toString()

/** Decides whether a failed API call is worth retrying. */
private fun retryOrFail(e: Exception, attempt: Int, maxAttempts: Int): androidx.work.ListenableWorker.Result = when {
    e is ApiException && e.status in 400..499 && e.status != 429 && e.status != 408 -> androidx.work.ListenableWorker.Result.failure()
    attempt >= maxAttempts -> androidx.work.ListenableWorker.Result.failure()
    else -> androidx.work.ListenableWorker.Result.retry()
}

/**
 * What an expedited worker shows while it runs as a foreground service,
 * which is how Android 8 to 11 run expedited work. Without this, those
 * versions drop the work on the floor.
 */
private fun syncForeground(context: Context, text: String): ForegroundInfo {
    val notification = Notifications.gateway(context, "simhook", text)
    return if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
        ForegroundInfo(Notifications.ID_SYNC, notification, ServiceInfo.FOREGROUND_SERVICE_TYPE_DATA_SYNC)
    } else {
        ForegroundInfo(Notifications.ID_SYNC, notification)
    }
}

// ---------------------------------------------------------------------------
// The outbox: fetched from the server, then sent
// ---------------------------------------------------------------------------

/** Fetches what the server holds for this phone and gets it sent. */
object OutboxSync {
    /** Returns how many messages are waiting to be sent afterwards. */
    suspend fun run(context: Context, container: AppContainer): Int {
        val items = container.api.outbox()
        if (items.isNotEmpty()) {
            val now = System.currentTimeMillis()
            container.outbox.insertAll(
                items.map {
                    OutboxMessage(id = it.id, batchId = it.batchId ?: "", to = it.to, body = it.body, simSubscriptionId = it.simSubscriptionId, createdAt = now)
                },
            )
        }
        // The server is reachable: anything owed to it goes now.
        ReportUploader.flush(container)
        val pending = container.outbox.pendingCount()
        if (pending > 0) GatewayService.startOrDrain(context)
        return pending
    }
}

/** Runs the outbox fetch when a push says there is something to send. */
class OutboxSyncWorker(context: Context, params: WorkerParameters) : CoroutineWorker(context, params) {
    override suspend fun getForegroundInfo(): ForegroundInfo = syncForeground(applicationContext, "Checking for messages to send")

    override suspend fun doWork(): Result {
        val container = SimhookApp.get(applicationContext).container
        if (!container.settings.current().isPaired) return Result.success()
        return try {
            OutboxSync.run(applicationContext, container)
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

    companion object {
        fun enqueue(context: Context) {
            val request = OneTimeWorkRequestBuilder<OutboxSyncWorker>()
                .setConstraints(networkConstraints)
                .setExpedited(OutOfQuotaPolicy.RUN_AS_NON_EXPEDITED_WORK_REQUEST)
                .setBackoffCriteria(BackoffPolicy.EXPONENTIAL, 15, TimeUnit.SECONDS)
                .build()
            // A push that arrives while a sync runs queues another one behind it.
            WorkManager.getInstance(context).enqueueUniqueWork("outbox-sync", ExistingWorkPolicy.APPEND_OR_REPLACE, request)
        }
    }
}

/**
 * Sends the outbox from a worker when the system refused to start the
 * foreground service. Bounded, because expedited work has a budget; what
 * is left queues another run behind this one. When the service is already
 * sending, the worker leaves the queue to it.
 */
class OutboxDrainWorker(context: Context, params: WorkerParameters) : CoroutineWorker(context, params) {
    override suspend fun getForegroundInfo(): ForegroundInfo = syncForeground(applicationContext, "Sending messages")

    override suspend fun doWork(): Result {
        val container = SimhookApp.get(applicationContext).container
        val remaining = OutboxDrainer.drain(applicationContext, container, deadlineMillis = System.currentTimeMillis() + BUDGET_MS, waitForTurn = false) {}
        if (remaining) enqueue(applicationContext, expedited = false)
        return Result.success()
    }

    companion object {
        private const val BUDGET_MS = 8 * 60_000L

        fun enqueue(context: Context, expedited: Boolean = true) {
            val builder = OneTimeWorkRequestBuilder<OutboxDrainWorker>()
                .setBackoffCriteria(BackoffPolicy.EXPONENTIAL, 15, TimeUnit.SECONDS)
            if (expedited) builder.setExpedited(OutOfQuotaPolicy.RUN_AS_NON_EXPEDITED_WORK_REQUEST)
            // Appended, never kept: a continuation is enqueued from inside the
            // running run, which a keep policy would silently discard.
            WorkManager.getInstance(context).enqueueUniqueWork("outbox-drain", ExistingWorkPolicy.APPEND_OR_REPLACE, builder.build())
        }
    }
}

// ---------------------------------------------------------------------------
// Heartbeat
// ---------------------------------------------------------------------------

class HeartbeatWorker(context: Context, params: WorkerParameters) : CoroutineWorker(context, params) {
    override suspend fun getForegroundInfo(): ForegroundInfo = syncForeground(applicationContext, "Checking in with the server")

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
            // A check-in also settles what the phone owes and picks up anything
            // waiting to be sent, so a phone that missed a push still sends
            // within one interval and a report never waits longer than one.
            runCatching { ReportUploader.flush(container) }
            runCatching { OutboxSync.run(applicationContext, container) }
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
// What the phone owes the server
// ---------------------------------------------------------------------------

/**
 * The ledger of reports the server has not taken yet. Every status and
 * every received text is written here first and uploaded from here, so
 * neither depends on the network being up at the moment it happened. The
 * upload is tried at once, again with backoff while it fails, and at every
 * check-in and outbox fetch regardless.
 */
object ReportLedger {
    suspend fun status(context: Context, container: AppContainer, messageId: String, status: String, at: Long, code: String?, message: String?) {
        container.reports.insert(
            PendingReport(kind = PendingReport.KIND_STATUS, messageId = messageId, status = status, errorCode = code, errorMessage = message, at = at, createdAt = System.currentTimeMillis()),
        )
        ReportUploadWorker.enqueue(context)
    }

    suspend fun inbound(context: Context, container: AppContainer, sender: String, body: String, receivedAt: Long, fingerprint: String, sim: Int?) {
        container.reports.insert(
            PendingReport(
                kind = PendingReport.KIND_INBOUND, sender = sender, body = body, fingerprint = fingerprint, simSubscriptionId = sim,
                at = receivedAt, createdAt = System.currentTimeMillis(),
            ),
        )
        ReportUploadWorker.enqueue(context)
    }
}

/** Uploads the ledger, oldest first. */
object ReportUploader {
    private const val PAGE = 50

    /**
     * Sends what it can and returns true when the ledger is empty. A report
     * the server refuses outright is dropped, since it will never take it;
     * anything else stops the run and waits for the next.
     */
    suspend fun flush(container: AppContainer): Boolean {
        val dao = container.reports
        while (true) {
            val batch = dao.oldest(PAGE)
            if (batch.isEmpty()) return true
            for (r in batch) {
                try {
                    send(container, r)
                    dao.delete(r.id)
                } catch (e: ApiException) {
                    if (e.isAuthFailure) {
                        // The pairing is gone, and with it everything owed under it.
                        container.handleLostPairing()
                        return true
                    }
                    if (e.status in 400..499 && e.status != 408 && e.status != 429) {
                        Log.w(TAG, "report ${r.id} (${r.kind}) refused by the server: ${e.code}")
                        dao.delete(r.id)
                        continue
                    }
                    dao.bump(r.id)
                    return false
                } catch (e: IOException) {
                    dao.bump(r.id)
                    return false
                }
            }
        }
    }

    private suspend fun send(container: AppContainer, r: PendingReport) {
        when (r.kind) {
            PendingReport.KIND_STATUS -> container.api.reportStatus(
                r.messageId ?: return,
                StatusReport(status = r.status ?: return, at = iso(r.at), errorCode = r.errorCode, errorMessage = r.errorMessage),
            )
            PendingReport.KIND_INBOUND -> container.api.reportInbound(
                InboundReport(r.sender ?: return, r.body ?: return, iso(r.at), r.fingerprint ?: return, r.simSubscriptionId),
            )
        }
    }
}

class ReportUploadWorker(context: Context, params: WorkerParameters) : CoroutineWorker(context, params) {
    override suspend fun getForegroundInfo(): ForegroundInfo = syncForeground(applicationContext, "Reporting to the server")

    override suspend fun doWork(): Result {
        val container = SimhookApp.get(applicationContext).container
        if (!container.settings.current().isPaired) return Result.success()
        // Never a failure: what is owed stays owed. The backoff grows while
        // the server is away, and the next check-in tries regardless.
        return if (ReportUploader.flush(container)) Result.success() else Result.retry()
    }

    companion object {
        fun enqueue(context: Context) {
            val request = OneTimeWorkRequestBuilder<ReportUploadWorker>()
                .setConstraints(networkConstraints)
                .setExpedited(OutOfQuotaPolicy.RUN_AS_NON_EXPEDITED_WORK_REQUEST)
                .setBackoffCriteria(BackoffPolicy.EXPONENTIAL, 10, TimeUnit.SECONDS)
                .build()
            // One uploader at a time; a run in progress reads the ledger again
            // before it finishes, so a report added meanwhile goes with it.
            WorkManager.getInstance(context).enqueueUniqueWork("report-upload", ExistingWorkPolicy.KEEP, request)
        }
    }
}

/**
 * Kept for work an earlier build left enqueued: its reports move into the
 * ledger instead of being sent from here.
 */
class StatusReportWorker(context: Context, params: WorkerParameters) : CoroutineWorker(context, params) {
    override suspend fun doWork(): Result {
        val id = inputData.getString("id") ?: return Result.failure()
        val status = inputData.getString("status") ?: return Result.failure()
        val container = SimhookApp.get(applicationContext).container
        ReportLedger.status(applicationContext, container, id, status, inputData.getLong("at", System.currentTimeMillis()), inputData.getString("code"), inputData.getString("message"))
        return Result.success()
    }
}

/** Kept for work an earlier build left enqueued; see [StatusReportWorker]. */
class InboundUploadWorker(context: Context, params: WorkerParameters) : CoroutineWorker(context, params) {
    override suspend fun doWork(): Result {
        val sender = inputData.getString("sender") ?: return Result.failure()
        val body = inputData.getString("body") ?: return Result.failure()
        val fingerprint = inputData.getString("fingerprint") ?: return Result.failure()
        val container = SimhookApp.get(applicationContext).container
        ReportLedger.inbound(
            applicationContext, container, sender, body, inputData.getLong("received_at", System.currentTimeMillis()), fingerprint,
            inputData.getInt("sim", -1).takeIf { it >= 0 },
        )
        return Result.success()
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
