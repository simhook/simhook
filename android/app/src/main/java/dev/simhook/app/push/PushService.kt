package dev.simhook.app.push

import com.google.firebase.messaging.FirebaseMessagingService
import com.google.firebase.messaging.RemoteMessage
import dev.simhook.app.SimhookApp
import dev.simhook.app.gateway.GatewayService
import dev.simhook.app.outbox.OutboxMessage
import dev.simhook.app.work.HeartbeatScheduler
import dev.simhook.app.work.PushTokenWorker
import kotlinx.coroutines.runBlocking

/**
 * The server wakes the phone with data pushes. Two kinds exist: a message to
 * send, and a request to check in.
 */
class PushService : FirebaseMessagingService() {
    override fun onMessageReceived(message: RemoteMessage) {
        val data = message.data
        when (data["type"]) {
            "send" -> {
                val id = data["message_id"] ?: return
                val batch = data["batch_id"] ?: ""
                val to = data["to"] ?: return
                val body = data["body"] ?: return
                val sim = data["sim_subscription_id"]?.toIntOrNull()
                val dao = SimhookApp.get(this).container.outbox
                runBlocking {
                    dao.insert(OutboxMessage(id = id, batchId = batch, to = to, body = body, simSubscriptionId = sim, createdAt = System.currentTimeMillis()))
                }
                GatewayService.start(this)
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
