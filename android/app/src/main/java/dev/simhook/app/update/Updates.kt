package dev.simhook.app.update

import android.app.DownloadManager
import android.app.PendingIntent
import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.net.Uri
import android.os.Environment
import android.util.Log
import androidx.work.BackoffPolicy
import androidx.work.Constraints
import androidx.work.CoroutineWorker
import androidx.work.ExistingPeriodicWorkPolicy
import androidx.work.NetworkType
import androidx.work.PeriodicWorkRequestBuilder
import androidx.work.WorkManager
import androidx.work.WorkerParameters
import dev.simhook.app.BuildConfig
import dev.simhook.app.SimhookApp
import dev.simhook.app.core.AppJson
import dev.simhook.app.core.AppVisibility
import dev.simhook.app.core.AvailableUpdate
import dev.simhook.app.core.Notifications
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import okhttp3.OkHttpClient
import okhttp3.Request
import java.io.File
import java.io.FileInputStream
import java.io.IOException
import java.security.MessageDigest
import java.util.concurrent.TimeUnit

private const val TAG = "Updates"

/**
 * The file published with every release. The app fetches it from
 * [BuildConfig.UPDATE_MANIFEST_URL] and offers the build it describes when
 * that build is newer than itself. See docs/decisions.md 012.
 */
@Serializable
data class UpdateManifest(
    @SerialName("version_code") val versionCode: Int,
    @SerialName("version_name") val versionName: String,
    /** Builds below this are told to update before things stop working. */
    @SerialName("min_supported_version_code") val minSupportedVersionCode: Int = 1,
    @SerialName("apk_url") val apkUrl: String,
    /** Hex SHA-256 of the APK; the download is discarded when it does not match. */
    @SerialName("sha256") val sha256: String,
    @SerialName("size_bytes") val sizeBytes: Long = 0L,
    @SerialName("released_at") val releasedAt: String? = null,
    val notes: String? = null,
)

object UpdateChecker {
    private const val STALE_AFTER_MS = 6 * 60 * 60 * 1000L

    private val http = OkHttpClient.Builder()
        .connectTimeout(15, TimeUnit.SECONDS)
        .readTimeout(20, TimeUnit.SECONDS)
        .build()

    /**
     * Looks for a newer build. Without [force], a check made in the last few
     * hours is reused. Returns the newer build, or null when this one is current.
     */
    suspend fun check(context: Context, force: Boolean): Result<AvailableUpdate?> = withContext(Dispatchers.IO) {
        val container = SimhookApp.get(context).container
        val settings = container.settings.current()
        if (!force && System.currentTimeMillis() - settings.updateCheckedAt < STALE_AFTER_MS) {
            return@withContext Result.success(settings.update?.takeIf { it.versionCode > BuildConfig.VERSION_CODE })
        }
        runCatching { fetch() }.mapCatching { manifest ->
            container.settings.setUpdateCheckedAt(System.currentTimeMillis())
            val update = manifest.takeIf { it.versionCode > BuildConfig.VERSION_CODE }?.let {
                AvailableUpdate(
                    versionCode = it.versionCode,
                    versionName = it.versionName,
                    apkUrl = it.apkUrl,
                    sha256 = it.sha256.lowercase(),
                    sizeBytes = it.sizeBytes,
                    notes = it.notes?.trim()?.takeIf { n -> n.isNotEmpty() },
                    required = it.minSupportedVersionCode > BuildConfig.VERSION_CODE,
                )
            }
            if (update != settings.update) {
                // Not the exact build we may have downloaded already.
                container.settings.setUpdateReadyUri(null)
            }
            container.settings.setAvailableUpdate(update)
            if (update == null) {
                Notifications.cancel(context, Notifications.ID_UPDATE_AVAILABLE)
            } else if (settings.updateNotifiedCode < update.versionCode) {
                Notifications.update(
                    context, Notifications.ID_UPDATE_AVAILABLE,
                    "simhook ${update.versionName} is available",
                    if (update.required) "This version is needed to keep the gateway working. Open the app to update." else "Open the app to update.",
                )
                container.settings.setUpdateNotifiedCode(update.versionCode)
            }
            update
        }.onFailure { Log.w(TAG, "update check failed", it) }
    }

    private fun fetch(): UpdateManifest {
        val request = Request.Builder()
            .url(BuildConfig.UPDATE_MANIFEST_URL)
            .header("User-Agent", "simhook-android/${BuildConfig.VERSION_NAME}")
            .header("Accept", "application/json")
            .header("Cache-Control", "no-cache")
            .build()
        http.newCall(request).execute().use { response ->
            if (!response.isSuccessful) throw IOException("update manifest returned HTTP ${response.code}")
            val manifest = AppJson.decodeFromString(UpdateManifest.serializer(), response.body.string())
            require(manifest.versionCode > 0) { "update manifest: bad version code" }
            require(manifest.apkUrl.startsWith("https://")) { "update manifest: apk url must use https" }
            require(manifest.sha256.length == 64 && manifest.sha256.all { it.isDigit() || it.lowercaseChar() in 'a'..'f' }) {
                "update manifest: bad sha256"
            }
            return manifest
        }
    }
}

/** Twice a day, in the background, so a phone nobody opens still learns about updates. */
class UpdateCheckWorker(context: Context, params: WorkerParameters) : CoroutineWorker(context, params) {
    override suspend fun doWork(): Result {
        val result = UpdateChecker.check(applicationContext, force = true)
        return when {
            result.isSuccess -> Result.success()
            runAttemptCount < 3 -> Result.retry()
            else -> Result.failure()
        }
    }
}

object UpdateScheduler {
    fun ensure(context: Context) {
        val request = PeriodicWorkRequestBuilder<UpdateCheckWorker>(12, TimeUnit.HOURS)
            .setConstraints(Constraints.Builder().setRequiredNetworkType(NetworkType.CONNECTED).build())
            .setBackoffCriteria(BackoffPolicy.EXPONENTIAL, 30, TimeUnit.MINUTES)
            .build()
        WorkManager.getInstance(context).enqueueUniquePeriodicWork("update-check", ExistingPeriodicWorkPolicy.KEEP, request)
    }
}

/**
 * Downloads a release through DownloadManager, checks its hash against the
 * manifest, and hands the file to the system package installer. The install
 * itself is Android's: it verifies the signature matches the installed app.
 */
object UpdateInstaller {
    private const val MIME = "application/vnd.android.package-archive"

    /** Starts the download. False when one is already running. */
    suspend fun start(context: Context, update: AvailableUpdate): Boolean = withContext(Dispatchers.IO) {
        val container = SimhookApp.get(context).container
        val settings = container.settings.current()
        val dm = context.getSystemService(DownloadManager::class.java) ?: return@withContext false
        if (settings.updateDownloadId >= 0 && isRunning(dm, settings.updateDownloadId)) return@withContext false
        val fileName = "simhook-${update.versionName}.apk"
        context.getExternalFilesDir(Environment.DIRECTORY_DOWNLOADS)?.let { File(it, fileName).delete() }
        val request = DownloadManager.Request(Uri.parse(update.apkUrl))
            .setTitle("simhook ${update.versionName}")
            .setDescription("Downloading the update")
            .setMimeType(MIME)
            .setNotificationVisibility(DownloadManager.Request.VISIBILITY_VISIBLE)
            .setDestinationInExternalFilesDir(context, Environment.DIRECTORY_DOWNLOADS, fileName)
        val id = dm.enqueue(request)
        container.settings.setUpdateDownloadId(id)
        container.settings.setUpdateReadyUri(null)
        Notifications.cancel(context, Notifications.ID_UPDATE_READY)
        true
    }

    /**
     * Opens the installer for an already downloaded and verified update.
     * False when there is none (or its file is gone), in which case the caller downloads.
     */
    suspend fun install(context: Context): Boolean = withContext(Dispatchers.IO) {
        val container = SimhookApp.get(context).container
        val raw = container.settings.current().updateReadyUri ?: return@withContext false
        val uri = Uri.parse(raw)
        val present = runCatching { context.contentResolver.openFileDescriptor(uri, "r")?.close() }.isSuccess
        if (!present) {
            container.settings.setUpdateReadyUri(null)
            return@withContext false
        }
        runCatching { context.startActivity(installIntent(uri)) }
            .onFailure { Log.w(TAG, "installer launch failed", it) }
            .isSuccess
    }

    /** DownloadManager finished our download: verify it, then open the installer or leave a notification. */
    suspend fun onDownloaded(context: Context, id: Long) = withContext(Dispatchers.IO) {
        val container = SimhookApp.get(context).container
        val settings = container.settings.current()
        if (id != settings.updateDownloadId) return@withContext
        container.settings.setUpdateDownloadId(-1L)
        val update = settings.update ?: return@withContext
        val dm = context.getSystemService(DownloadManager::class.java) ?: return@withContext

        val status = status(dm, id)
        if (status != DownloadManager.STATUS_SUCCESSFUL) {
            Log.w(TAG, "update download $id ended with status $status")
            Notifications.update(context, Notifications.ID_UPDATE_READY, "Update download failed", "Open the app to try again.")
            return@withContext
        }
        val digest = runCatching { sha256(dm, id) }.getOrNull()
        if (digest == null || !digest.equals(update.sha256, ignoreCase = true)) {
            Log.w(TAG, "update download $id hash mismatch: got $digest, want ${update.sha256}")
            dm.remove(id)
            Notifications.update(
                context, Notifications.ID_UPDATE_READY,
                "Update could not be verified",
                "The downloaded file did not match the published checksum and was discarded. Open the app to try again.",
            )
            // Re-read the manifest so a corrected release fixes this on the next try.
            UpdateChecker.check(context, force = true)
            return@withContext
        }

        val uri = dm.getUriForDownloadedFile(id)
        if (uri == null) {
            Log.w(TAG, "update download $id has no content uri")
            return@withContext
        }
        // Keep the verified file on offer, so granting the install permission or dismissing
        // the installer does not cost another download.
        container.settings.setUpdateReadyUri(uri.toString())
        val intent = installIntent(uri)
        // Android only lets a visible app open the installer; otherwise the notification carries it.
        val opened = AppVisibility.visible && runCatching { context.startActivity(intent) }.isSuccess
        if (!opened) {
            val pending = PendingIntent.getActivity(
                context, 1, intent,
                PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE,
            )
            Notifications.update(
                context, Notifications.ID_UPDATE_READY,
                "simhook ${update.versionName} is ready to install",
                "Tap to install the update.",
                pending,
            )
        }
    }

    private fun installIntent(uri: Uri): Intent = Intent(Intent.ACTION_VIEW)
        .setDataAndType(uri, MIME)
        .addFlags(Intent.FLAG_ACTIVITY_NEW_TASK or Intent.FLAG_GRANT_READ_URI_PERMISSION)

    private fun isRunning(dm: DownloadManager, id: Long): Boolean = when (status(dm, id)) {
        DownloadManager.STATUS_PENDING, DownloadManager.STATUS_RUNNING, DownloadManager.STATUS_PAUSED -> true
        else -> false
    }

    private fun status(dm: DownloadManager, id: Long): Int {
        dm.query(DownloadManager.Query().setFilterById(id)).use { c ->
            if (!c.moveToFirst()) return -1
            return c.getInt(c.getColumnIndexOrThrow(DownloadManager.COLUMN_STATUS))
        }
    }

    private fun sha256(dm: DownloadManager, id: Long): String {
        val digest = MessageDigest.getInstance("SHA-256")
        dm.openDownloadedFile(id).use { pfd ->
            FileInputStream(pfd.fileDescriptor).use { input ->
                val buffer = ByteArray(64 * 1024)
                while (true) {
                    val n = input.read(buffer)
                    if (n < 0) break
                    digest.update(buffer, 0, n)
                }
            }
        }
        return digest.digest().joinToString("") { "%02x".format(it) }
    }
}

/** Receives DownloadManager's completion broadcast for update downloads. */
class UpdateDownloadReceiver : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        if (intent.action != DownloadManager.ACTION_DOWNLOAD_COMPLETE) return
        val id = intent.getLongExtra(DownloadManager.EXTRA_DOWNLOAD_ID, -1L)
        if (id < 0) return
        val pending = goAsync()
        val app = context.applicationContext
        CoroutineScope(Dispatchers.IO).launch {
            try {
                UpdateInstaller.onDownloaded(app, id)
            } catch (e: Exception) {
                Log.w(TAG, "handling download $id failed", e)
            } finally {
                pending.finish()
            }
        }
    }
}
