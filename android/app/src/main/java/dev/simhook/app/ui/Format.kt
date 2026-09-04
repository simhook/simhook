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
