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
    /** Reachability of the control server while the node is not running yet: unknown, ok or unreachable. */
    val control: String = CONTROL_UNKNOWN,
    @SerializedName("control_error") val controlError: String = "",
) {
    val isRunning get() = state == RUNNING
    val needsLogin get() = state == NEEDS_LOGIN && authUrl.isNotEmpty()

    /** The node is waiting for control and it is not answering (refused, no route, or timeouts for a long time). */
    val controlUnreachable get() = !isRunning && control == CONTROL_UNREACHABLE

    /** Probes to control time out so far: a slow path (i2pd, a proxy) that may still come up. */
    val controlSlow get() = !isRunning && control == CONTROL_SLOW

    companion object {
        const val STOPPED = "stopped"
        const val RUNNING = "running"
        const val NEEDS_LOGIN = "needs-login"
        const val CONTROL_UNKNOWN = "unknown"
        const val CONTROL_UNREACHABLE = "unreachable"
        const val CONTROL_SLOW = "slow"

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
            return TailnetStatus(parsed.state ?: STOPPED, parsed.authUrl ?: "", parsed.ips ?: emptyList(), parsed.updatedAt, parsed.control ?: CONTROL_UNKNOWN, parsed.controlError ?: "")
        }
    }
}
