package dev.simhook.app.push

import com.google.firebase.messaging.FirebaseMessagingService
import com.google.firebase.messaging.RemoteMessage
import dev.simhook.app.SimhookApp
import dev.simhook.app.work.HeartbeatScheduler
import dev.simhook.app.work.OutboxSyncWorker
import dev.simhook.app.work.PushTokenWorker
import kotlinx.coroutines.runBlocking

/**
 * The server wakes the phone with data pushes that carry no content. Two
 * kinds exist: "there is something to send", naming the phone, after which
 * the phone fetches its outbox; and "check in".
 */
class PushService : FirebaseMessagingService() {
    override fun onMessageReceived(message: RemoteMessage) {
        val data = message.data
        when (data["type"]) {
            "send" -> {
                val settings = runBlocking { SimhookApp.get(this@PushService).container.settings.current() }
                if (!settings.isPaired) return
                // A push for a pairing this phone no longer holds is not ours to act on.
                val forDevice = data["device_id"]
                if (forDevice != null && forDevice != settings.deviceId) return
                OutboxSyncWorker.enqueue(this)
            }
            "heartbeat" -> HeartbeatScheduler.runNow(this)
        }
    }

    /** Newer SDKs report registrations here; older ones through onNewToken. Both mean the same thing. */
    override fun onRegistered(token: String) {
        tokenChanged(token)
    }

    @Deprecated("Kept for SDKs that still call it", ReplaceWith("onRegistered(token)"))
    override fun onNewToken(token: String) {
        tokenChanged(token)
    }

    private fun tokenChanged(token: String) {
        val container = SimhookApp.get(this).container
        runBlocking { container.settings.setPushToken(token) }
        PushTokenWorker.enqueue(this, token)
    }
}
