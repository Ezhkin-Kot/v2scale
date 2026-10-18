package com.v2ray.ang.ui.main

import com.v2ray.ang.dto.TailnetStatus
import com.v2ray.ang.dto.TailnetTestResult
import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Test

class MainTailnetTest {
    private val running = TailnetSnapshot(true, TailnetStatus(TailnetStatus.RUNNING, "", listOf("100.64.0.13")))
    private val starting = TailnetSnapshot(true, TailnetStatus("starting"))
    private val login = TailnetSnapshot(true, TailnetStatus(TailnetStatus.NEEDS_LOGIN, "https://hs.example.net/x"))
    private val none = TailnetTestState.None

    @Test
    fun hiddenWhenTailnetIsOffOrTheCoreIsStopped() {
        assertEquals(MainTailnet.Hidden, mainTailnetOf(true, TailnetSnapshot(false), none))
        assertEquals(MainTailnet.Hidden, mainTailnetOf(false, running, none))
        assertEquals(MainTailnet.Hidden, mainTailnetOf(false, running, TailnetTestState.Done(TailnetTestResult(ok = true))))
    }

    @Test
    fun followsTheNodeStateWithoutATest() {
        assertEquals(MainTailnet.Ready(listOf("100.64.0.13")), mainTailnetOf(true, running, none))
        assertEquals(MainTailnet.Connecting, mainTailnetOf(true, starting, none))
        assertEquals(MainTailnet.Connecting, mainTailnetOf(true, TailnetSnapshot(true), none))
        assertEquals(MainTailnet.NeedsLogin, mainTailnetOf(true, login, none))
    }

    @Test
    fun aTestOverridesTheNodeStateUntilItIsDone() {
        assertEquals(MainTailnet.Testing, mainTailnetOf(true, running, TailnetTestState.Running("r1")))
        val failed = TailnetTestResult(ok = false, state = "starting", error = "node is starting")
        assertEquals(MainTailnet.Tested(failed), mainTailnetOf(true, starting, TailnetTestState.Done(failed)))
    }

    @Test
    fun anUnreachableControlServerIsShownWhileWaitingForTheNode() {
        val down = TailnetSnapshot(true, TailnetStatus("starting", control = TailnetStatus.CONTROL_UNREACHABLE, controlError = "i/o timeout"))
        assertEquals(MainTailnet.ControlUnreachable("i/o timeout"), mainTailnetOf(true, down, none))
    }

    @Test
    fun aRunningNodeIgnoresALateUnreachableReport() {
        val late = TailnetSnapshot(true, TailnetStatus(TailnetStatus.RUNNING, "", listOf("100.64.0.13"), control = TailnetStatus.CONTROL_UNREACHABLE))
        assertEquals(MainTailnet.Ready(listOf("100.64.0.13")), mainTailnetOf(true, late, none))
    }

    @Test
    fun loginWithoutALinkIsJustConnecting() {
        val noLink = TailnetSnapshot(true, TailnetStatus(TailnetStatus.NEEDS_LOGIN, ""))
        assertEquals(MainTailnet.Connecting, mainTailnetOf(true, noLink, none))
    }
}
