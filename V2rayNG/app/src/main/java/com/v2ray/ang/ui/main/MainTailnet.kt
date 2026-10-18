package com.v2ray.ang.ui.main

import com.v2ray.ang.dto.TailnetStatus
import com.v2ray.ang.dto.TailnetTestResult

/** Tailnet line of the main screen, locale-neutral until it reaches the UI. */
sealed interface MainTailnet {
    /** Tailnet is off or the core is not running: nothing to show. */
    data object Hidden : MainTailnet
    data object Connecting : MainTailnet
    data object NeedsLogin : MainTailnet
    data class Ready(val ips: List<String>) : MainTailnet
    data object Testing : MainTailnet
    data class Tested(val result: TailnetTestResult) : MainTailnet
}

/** What the user has done on the tailnet line since the core started. */
sealed interface TailnetTestState {
    data object None : TailnetTestState
    data class Running(val requestId: String) : TailnetTestState
    data class Done(val result: TailnetTestResult) : TailnetTestState
}

/** Enabled flag and node state, read from settings storage where the daemon process publishes them. */
data class TailnetSnapshot(val enabled: Boolean = false, val status: TailnetStatus = TailnetStatus())

fun mainTailnetOf(isRunning: Boolean, snapshot: TailnetSnapshot, test: TailnetTestState): MainTailnet {
    if (!isRunning || !snapshot.enabled) return MainTailnet.Hidden
    return when (test) {
        is TailnetTestState.Running -> MainTailnet.Testing
        is TailnetTestState.Done -> MainTailnet.Tested(test.result)
        TailnetTestState.None -> when {
            snapshot.status.isRunning -> MainTailnet.Ready(snapshot.status.ips)
            snapshot.status.needsLogin -> MainTailnet.NeedsLogin
            else -> MainTailnet.Connecting
        }
    }
}
