package dev.simhook.app.core

/** Whether the app has a visible activity. Background code uses it to decide between opening a screen and posting a notification. */
object AppVisibility {
    @Volatile
    var visible: Boolean = false
}
