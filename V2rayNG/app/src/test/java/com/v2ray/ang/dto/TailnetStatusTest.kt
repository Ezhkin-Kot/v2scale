package com.v2ray.ang.dto

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class TailnetStatusTest {
    @Test
    fun parsesRunningNode() {
        val s = TailnetStatus.parse("""{"state":"running","auth_url":"","ips":["100.64.0.12"]}""")
        assertTrue(s.isRunning)
        assertFalse(s.needsLogin)
        assertEquals(listOf("100.64.0.12"), s.ips)
    }

    @Test
    fun parsesLoginRequest() {
        val s = TailnetStatus.parse("""{"state":"needs-login","auth_url":"https://hs.example.net/register/x","ips":[]}""")
        assertTrue(s.needsLogin)
        assertEquals("https://hs.example.net/register/x", s.authUrl)
    }

    @Test
    fun needsLoginWithoutLinkIsNotActionable() {
        assertFalse(TailnetStatus.parse("""{"state":"needs-login"}""").needsLogin)
    }

    @Test
    fun missingFieldsFallBackToDefaults() {
        val s = TailnetStatus.parse("{}")
        assertEquals(TailnetStatus.STOPPED, s.state)
        assertEquals(emptyList<String>(), s.ips)
    }

    @Test
    fun aRecentStatusIsKeptAndAnOldOrUndatedOneIsStopped() {
        val running = TailnetStatus(TailnetStatus.RUNNING, "", listOf("100.64.0.13"), updatedAt = 100_000)
        assertEquals(running, TailnetStatus.fresh(running, now = 105_000))
        assertEquals(running, TailnetStatus.fresh(running, now = 110_000))
        assertEquals(TailnetStatus(), TailnetStatus.fresh(running, now = 110_001))
        assertEquals(TailnetStatus(), TailnetStatus.fresh(running.copy(updatedAt = 0), now = 1))
    }

    @Test
    fun parsesThePublishTime() {
        assertEquals(42L, TailnetStatus.parse("""{"state":"running","updatedAt":42}""").updatedAt)
    }

    @Test
    fun blankAndMalformedInputIsStopped() {
        assertEquals(TailnetStatus(), TailnetStatus.parse(null))
        assertEquals(TailnetStatus(), TailnetStatus.parse(" "))
        assertEquals(TailnetStatus(), TailnetStatus.parse("{not json"))
    }
}
