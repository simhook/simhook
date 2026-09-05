package dev.simhook.app.ui

import android.text.format.DateUtils
import java.time.Instant

/** "3 min. ago", "Yesterday", and so on. */
fun relativeTime(millis: Long, now: Long = System.currentTimeMillis()): String {
    if (millis <= 0) return "never"
    return DateUtils.getRelativeTimeSpanString(millis, now, DateUtils.MINUTE_IN_MILLIS, DateUtils.FORMAT_ABBREV_RELATIVE).toString()
}

fun parseInstant(iso: String?): Long? = iso?.let { runCatching { Instant.parse(it).toEpochMilli() }.getOrNull() }

fun statusLabel(status: String): String = when (status) {
    "queued" -> "Queued"
    "dispatched" -> "On the phone"
    "sent" -> "Sent"
    "delivered" -> "Delivered"
    "failed" -> "Failed"
    "unknown" -> "No result"
    "received" -> "Received"
    else -> status.replaceFirstChar { it.uppercase() }
}

/** "8.4 MB", "512 KB". */
fun formatBytes(bytes: Long): String = when {
    bytes >= 1_048_576 -> String.format(java.util.Locale.US, "%.1f MB", bytes / 1_048_576.0)
    bytes >= 1_024 -> "${bytes / 1_024} KB"
    else -> "$bytes B"
}
