package dev.simhook.app.core

import kotlinx.serialization.json.Json

/** One JSON configuration for the whole app: lenient on unknown fields so the API can grow. */
val AppJson: Json = Json {
    ignoreUnknownKeys = true
    explicitNulls = false
    encodeDefaults = false
    isLenient = true
}
