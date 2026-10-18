package com.v2ray.ang.dto

import com.google.gson.annotations.SerializedName
import com.v2ray.ang.util.JsonUtil
import java.io.Serializable

/** Outcome of the tailnet connection test, as reported by the core (`CoreController.tailnetTest`). */
data class TailnetTestResult(
    val ok: Boolean = false,
    val state: String = "",
    @SerializedName("latency_ms") val latencyMillis: Long = 0,
    /** "direct", "relay" or "tcp". */
    val path: String = "",
    val peer: String = "",
    val error: String = "",
) : Serializable {
    companion object {
        /** Unparseable input is a failed test with a reason, never an exception. */
        fun parse(json: String?): TailnetTestResult {
            val parsed = json?.takeIf { it.isNotBlank() }
                ?.let { runCatching { JsonUtil.fromJsonSafe(it, TailnetTestResult::class.java) }.getOrNull() }
                ?: return TailnetTestResult(error = "no result")
            // Gson bypasses defaults for missing fields, so normalize nulls.
            @Suppress("USELESS_ELVIS")
            return TailnetTestResult(parsed.ok, parsed.state ?: "", parsed.latencyMillis, parsed.path ?: "", parsed.peer ?: "", parsed.error ?: "")
        }
    }
}
