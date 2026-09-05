package dev.simhook.app.core

import java.net.URI
import java.net.URLDecoder

/**
 * A pairing link: simhook://pair?code=XXXX-XXXX&api=https://api.example.com
 *
 * The code is what the dashboard shows; the api part lets a self-hosted
 * dashboard point the app at its own server. Only an https address is
 * accepted there, plus plain http on a loopback address for development,
 * because the app would otherwise hand its device token to whatever the
 * link named.
 */
data class PairLink(val code: String, val api: String?) {
    /** The server's host name, for asking the person before pairing with a server that is not the default. */
    val host: String? get() = api?.let { runCatching { URI(it).host }.getOrNull() }

    companion object {
        fun parse(raw: String?): PairLink? {
            val text = raw?.trim()?.takeIf { it.isNotEmpty() } ?: return null
            val uri = runCatching { URI(text) }.getOrNull() ?: return null
            if (!uri.scheme.equals("simhook", ignoreCase = true) || !uri.host.equals("pair", ignoreCase = true)) return null
            val query = queryOf(uri.rawQuery)
            val code = query["code"]?.trim()?.takeIf { it.isNotEmpty() } ?: return null
            val api = query["api"]?.trim()?.trimEnd('/')?.takeIf { allowedApi(it) }
            return PairLink(code, api)
        }

        /** Whether the app may talk to this API address. */
        fun allowedApi(url: String): Boolean {
            val uri = runCatching { URI(url.trim()) }.getOrNull() ?: return false
            val host = uri.host?.lowercase() ?: return false
            return when (uri.scheme?.lowercase()) {
                "https" -> true
                "http" -> host == "localhost" || host == "127.0.0.1" || host == "10.0.2.2" || host == "[::1]"
                else -> false
            }
        }

        private fun queryOf(raw: String?): Map<String, String> {
            if (raw.isNullOrEmpty()) return emptyMap()
            return raw.split('&').mapNotNull { pair ->
                val i = pair.indexOf('=')
                if (i <= 0) return@mapNotNull null
                val key = decode(pair.substring(0, i))
                val value = decode(pair.substring(i + 1))
                key to value
            }.toMap()
        }

        private fun decode(s: String): String = runCatching { URLDecoder.decode(s, "UTF-8") }.getOrDefault(s)
    }
}
