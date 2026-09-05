package dev.simhook.app.gateway

import android.app.Service
import android.content.Context
import android.content.Intent
import android.content.pm.ServiceInfo
import android.os.Build
import android.os.IBinder
import android.util.Log
import androidx.core.app.ServiceCompat
import androidx.core.content.ContextCompat
import dev.simhook.app.SimhookApp
import dev.simhook.app.core.Notifications
import dev.simhook.app.outbox.OutboxDrainer
import dev.simhook.app.work.OutboxDrainWorker
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.launch

/**
 * The foreground service that does the actual sending. It runs while the
 * outbox has work, and stays up as a keep-alive when the user asks for it,
 * which stops aggressive phones from freezing the app.
 */
class GatewayService : Service() {
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.Default)
    private var drainer: Job? = null

    override fun onBind(intent: Intent?): IBinder? = null

    override fun onCreate() {
        super.onCreate()
        showNotification("Gateway ready", "Waiting for messages to send.")
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        if (drainer?.isActive != true) {
            drainer = scope.launch { drain() }
        }
        return START_STICKY
    }

    override fun onDestroy() {
        scope.cancel()
        super.onDestroy()
    }

    private fun showNotification(title: String, text: String) {
        val notification = Notifications.gateway(this, title, text)
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) {
            ServiceCompat.startForeground(this, Notifications.ID_GATEWAY, notification, ServiceInfo.FOREGROUND_SERVICE_TYPE_REMOTE_MESSAGING)
        } else {
            startForeground(Notifications.ID_GATEWAY, notification)
        }
    }

    private suspend fun drain() {
        val container = SimhookApp.get(this).container
        OutboxDrainer.drain(this, container, deadlineMillis = null) { remaining ->
            showNotification("Sending messages", if (remaining == 1) "1 message in the queue" else "$remaining messages in the queue")
        }
        val settings = container.settings.current()
        if (settings.keepAliveNotification && settings.isPaired) {
            showNotification("Gateway active", "Ready to send and receive.")
            return // stay alive; the next push starts a new drain
        }
        stopSelf()
    }

    companion object {
        private const val TAG = "GatewayService"

        /**
         * Starts (or nudges) the service. False when the system refused a
         * foreground start from the background, which newer Android versions
         * do outside a few windows; the caller then sends another way.
         */
        fun start(context: Context): Boolean = try {
            ContextCompat.startForegroundService(context, Intent(context, GatewayService::class.java))
            true
        } catch (e: Exception) {
            Log.w(TAG, "foreground start refused", e)
            false
        }

        /** Starts the service, or drains the outbox from a worker when the service cannot start. */
        fun startOrDrain(context: Context) {
            if (!start(context)) OutboxDrainWorker.enqueue(context)
        }

        fun stop(context: Context) {
            context.stopService(Intent(context, GatewayService::class.java))
        }
    }
}
