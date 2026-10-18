package com.v2ray.ang.core

import android.content.Context
import com.v2ray.ang.AppConfig
import com.v2ray.ang.dto.TailnetStatus
import com.v2ray.ang.dto.TailnetTestResult
import com.v2ray.ang.handler.MmkvManager
import com.v2ray.ang.handler.TailnetStore
import com.v2ray.ang.util.JsonUtil
import com.v2ray.ang.util.LogUtil
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.delay
import kotlinx.coroutines.isActive
import kotlinx.coroutines.launch
import libv2ray.CoreController
import java.io.File

/**
 * Runs in the daemon process next to the core: hands the tailnet node config to the core before it
 * starts and mirrors the node state into settings storage, which is how the UI process sees it.
 * [scope] belongs to CoreServiceManager and is cancelled with the service.
 */
class TailnetRuntime(
    private val controller: CoreController,
    private val scope: CoroutineScope,
) {
    private companion object {
        const val STATUS_POLL_MS = 2000L
        const val STATE_DIR = "tailnet"
    }

    private var statusJob: Job? = null

    /**
     * Configures (or clears) the node for the next `startLoop`.
     * @throws IllegalStateException when tailnet is enabled with settings that cannot start.
     */
    fun configure(context: Context) {
        val settings = TailnetStore.load()
        if (!settings.enabled) {
            controller.setTailnetConfig("")
            return
        }
        val problem = TailnetConfigBuilder.validate(settings)
        check(problem == null) { "Tailnet settings are invalid: $problem" }
        val stateDir = File(context.filesDir, STATE_DIR).apply { mkdirs() }.absolutePath
        controller.setTailnetConfig(TailnetConfigBuilder.nodeJson(settings, stateDir))
    }

    /** Starts mirroring the node state; replaces a previous loop, as after a reload. */
    fun startStatusLoop() {
        statusJob?.cancel()
        if (!TailnetStore.load().enabled) {
            publish(TailnetStatus())
            return
        }
        statusJob = scope.launch {
            while (isActive) {
                try {
                    val status = TailnetStatus.parse(controller.tailnetStatus())
                    publish(status)
                    if (status.isRunning) TailnetStore.clearAuthKey()
                } catch (e: CancellationException) {
                    throw e
                } catch (e: Exception) {
                    LogUtil.w(AppConfig.TAG, "Tailnet: status poll failed", e)
                }
                delay(STATUS_POLL_MS)
            }
        }
    }

    /**
     * Runs the connection test off the caller's thread and hands the result to [onResult].
     * The core may stop meanwhile, in which case the result describes a stopped node.
     */
    fun runTest(onResult: (TailnetTestResult) -> Unit) {
        scope.launch {
            val target = TailnetStore.load().testTarget
            val result = try {
                TailnetTestResult.parse(controller.tailnetTest(target))
            } catch (e: CancellationException) {
                throw e
            } catch (e: Exception) {
                LogUtil.w(AppConfig.TAG, "Tailnet: connection test failed", e)
                TailnetTestResult(error = e.message ?: e.javaClass.simpleName)
            }
            onResult(result)
        }
    }

    fun stopStatusLoop() {
        statusJob?.cancel()
        statusJob = null
        publish(TailnetStatus())
    }

    private fun publish(status: TailnetStatus) {
        MmkvManager.encodeSettings(AppConfig.PREF_TAILNET_STATUS, JsonUtil.toJson(status.copy(updatedAt = System.currentTimeMillis())))
    }
}
