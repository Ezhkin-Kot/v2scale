package com.v2ray.ang.dto

import com.google.gson.annotations.SerializedName
import com.v2ray.ang.util.JsonUtil

/** Node state reported by the core (`CoreController.tailnetStatus()`), mirrored into settings storage for the UI. */
data class TailnetStatus(
    val state: String = STOPPED,
    @SerializedName("auth_url") val authUrl: String = "",
    val ips: List<String> = emptyList(),
    /** Epoch millis when the daemon published this status, 0 when unknown. */
    val updatedAt: Long = 0,
) {
    val isRunning get() = state == RUNNING
    val needsLogin get() = state == NEEDS_LOGIN && authUrl.isNotEmpty()

    companion object {
        const val STOPPED = "stopped"
        const val RUNNING = "running"
        const val NEEDS_LOGIN = "needs-login"

        /** A published status older than this belongs to a daemon that is gone. */
        const val STALE_AFTER_MS = 10_000L

        /** [status] itself when the daemon published it recently, otherwise a stopped node. */
        fun fresh(status: TailnetStatus, now: Long = System.currentTimeMillis()): TailnetStatus =
            if (status.updatedAt > 0 && now - status.updatedAt <= STALE_AFTER_MS) status else TailnetStatus()

        /** Unparseable or empty input is a stopped node, never an error. */
        fun parse(json: String?): TailnetStatus {
            if (json.isNullOrBlank()) return TailnetStatus()
            val parsed = runCatching { JsonUtil.fromJsonSafe(json, TailnetStatus::class.java) }.getOrNull()
                ?: return TailnetStatus()
            // Gson bypasses defaults for missing fields, so normalize nulls.
            @Suppress("USELESS_ELVIS")
            return TailnetStatus(parsed.state ?: STOPPED, parsed.authUrl ?: "", parsed.ips ?: emptyList(), parsed.updatedAt)
        }
    }
}
