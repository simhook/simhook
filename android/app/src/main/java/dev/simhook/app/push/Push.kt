package dev.simhook.app.push

import android.content.Context
import com.google.firebase.FirebaseApp
import com.google.firebase.messaging.FirebaseMessaging
import kotlinx.coroutines.tasks.await
import kotlinx.coroutines.withTimeoutOrNull

/** Push availability depends on the Firebase config compiled into the build. */
object Push {
    fun available(context: Context): Boolean = FirebaseApp.getApps(context).isNotEmpty()

    /**
     * The current registration token, or null when push is unavailable, not
     * yet registered, or slow to answer. Callers never wait on push: a token
     * that arrives later is reported through [PushService].
     */
    suspend fun token(context: Context, timeoutMs: Long = 5_000): String? {
        if (!available(context)) return null
        @Suppress("DEPRECATION")
        return withTimeoutOrNull(timeoutMs) {
            runCatching { FirebaseMessaging.getInstance().token.await() }.getOrNull()
        }
    }
}
