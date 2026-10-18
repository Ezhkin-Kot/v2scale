package com.v2ray.ang.dto

import com.google.gson.annotations.SerializedName
import com.v2ray.ang.util.JsonUtil

/** Node state reported by the core (`CoreController.tailnetStatus()`), mirrored into settings storage for the UI. */
data class TailnetStatus(
    val state: String = STOPPED,
    @SerializedName("auth_url") val authUrl: String = "",
    val ips: List<String> = emptyList(),
) {
    val isRunning get() = state == RUNNING
    val needsLogin get() = state == NEEDS_LOGIN && authUrl.isNotEmpty()

    companion object {
        const val STOPPED = "stopped"
        const val RUNNING = "running"
        const val NEEDS_LOGIN = "needs-login"

        /** Unparseable or empty input is a stopped node, never an error. */
        fun parse(json: String?): TailnetStatus {
            if (json.isNullOrBlank()) return TailnetStatus()
            val parsed = runCatching { JsonUtil.fromJsonSafe(json, TailnetStatus::class.java) }.getOrNull()
                ?: return TailnetStatus()
            // Gson bypasses defaults for missing fields, so normalize nulls.
            @Suppress("USELESS_ELVIS")
            return TailnetStatus(parsed.state ?: STOPPED, parsed.authUrl ?: "", parsed.ips ?: emptyList())
        }
    }
}
