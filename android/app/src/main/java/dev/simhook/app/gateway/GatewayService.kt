package dev.simhook.app.gateway

import android.app.Service
import android.content.Context
import android.content.Intent
import android.content.pm.ServiceInfo
import android.os.Build
import android.os.IBinder
import androidx.core.app.ServiceCompat
import androidx.core.content.ContextCompat
import dev.simhook.app.SimhookApp
import dev.simhook.app.core.Notifications
import dev.simhook.app.outbox.OutboxMessage
import dev.simhook.app.sms.SendTracker
import dev.simhook.app.sms.SmsSender
import dev.simhook.app.work.StatusReportWorker
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.Job
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.cancel
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlinx.coroutines.withTimeoutOrNull

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
        val dao = container.outbox
        val now = System.currentTimeMillis()

        // Anything left mid-send by a previous process life is reported as
        // failed with an honest reason, since we cannot know whether the
        // radio sent it.
        for (stuck in dao.interrupted(before = now - 60_000)) {
            dao.setState(stuck.id, OutboxMessage.STATE_FAILED, now, "Interrupted while sending.")
            StatusReportWorker.enqueue(this, stuck.id, "failed", now, "interrupted", "The app was interrupted while sending. The message may or may not have gone out.")
        }
        dao.prune(before = now - 7L * 24 * 3600 * 1000)

        while (true) {
            val settings = container.settings.current()
            val next = if (settings.isPaired && settings.gatewayEnabled) dao.nextPending() else null
            if (next == null) {
                if (settings.keepAliveNotification && settings.isPaired) {
                    showNotification("Gateway active", "Ready to send and receive.")
                    return // stay alive; the next push starts a new drain
                }
                stopSelf()
                return
            }
            val remaining = dao.inFlightCount()
            showNotification("Sending messages", if (remaining == 1) "1 message in the queue" else "$remaining messages in the queue")

            val sim = next.simSubscriptionId ?: settings.preferredSimSubscriptionId
            val t = System.currentTimeMillis()
            val waiter = SendTracker.expect(next.id)
            when (val outcome = SmsSender.send(this, next.id, next.to, next.body, sim)) {
                is SmsSender.Outcome.Failed -> {
                    SendTracker.forget(next.id)
                    dao.markSending(next.id, 1, t)
                    dao.setState(next.id, OutboxMessage.STATE_FAILED, t, outcome.failure.message)
                    StatusReportWorker.enqueue(this, next.id, "failed", t, outcome.failure.code, outcome.failure.message)
                }
                is SmsSender.Outcome.Handed -> {
                    dao.markSending(next.id, outcome.parts, t)
                    dao.setState(next.id, OutboxMessage.STATE_HANDED, t, null)
                    // Wait for the radio's verdict so pacing counts real sends.
                    withTimeoutOrNull(SENT_TIMEOUT_MS) { waiter.await() }
                    SendTracker.forget(next.id)
                }
            }
            delay(settings.sendDelaySeconds.coerceIn(0, 3600) * 1000L)
        }
    }

    companion object {
        private const val SENT_TIMEOUT_MS = 45_000L

        /** Start (or nudge) the service. Safe to call from any context that may start foreground services. */
        fun start(context: Context) {
            runCatching { ContextCompat.startForegroundService(context, Intent(context, GatewayService::class.java)) }
        }

        fun stop(context: Context) {
            context.stopService(Intent(context, GatewayService::class.java))
        }
    }
}
