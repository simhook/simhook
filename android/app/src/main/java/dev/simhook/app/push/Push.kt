package dev.simhook.app.push

import android.content.Context
import com.google.firebase.FirebaseApp
import com.google.firebase.messaging.FirebaseMessaging
import kotlinx.coroutines.tasks.await

/** Push availability depends on the Firebase config compiled into the build. */
object Push {
    fun available(context: Context): Boolean = FirebaseApp.getApps(context).isNotEmpty()

    /** The current registration token, or null when push is unavailable or not yet registered. */
    suspend fun token(context: Context): String? {
        if (!available(context)) return null
        @Suppress("DEPRECATION")
        return runCatching { FirebaseMessaging.getInstance().token.await() }.getOrNull()
    }
}
