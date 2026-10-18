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
    fun blankAndMalformedInputIsStopped() {
        assertEquals(TailnetStatus(), TailnetStatus.parse(null))
        assertEquals(TailnetStatus(), TailnetStatus.parse(" "))
        assertEquals(TailnetStatus(), TailnetStatus.parse("{not json"))
    }
}
