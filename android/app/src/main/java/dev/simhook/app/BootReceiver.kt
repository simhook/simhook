package dev.simhook.app

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import dev.simhook.app.gateway.GatewayService
import dev.simhook.app.work.HeartbeatScheduler
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.launch

/** After a reboot, get back to work without waiting for the user to open the app. */
class BootReceiver : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        if (intent.action != Intent.ACTION_BOOT_COMPLETED && intent.action != Intent.ACTION_MY_PACKAGE_REPLACED) return
        val pending = goAsync()
        val app = context.applicationContext
        CoroutineScope(Dispatchers.IO).launch {
            try {
                val container = SimhookApp.get(app).container
                val settings = container.settings.current()
                if (!settings.isPaired) return@launch
                HeartbeatScheduler.ensure(app, settings.heartbeatIntervalMinutes)
                HeartbeatScheduler.runNow(app)
                if (settings.keepAliveNotification || container.outbox.inFlightCount() > 0) {
                    GatewayService.start(app)
                }
            } finally {
                pending.finish()
            }
        }
    }
}
