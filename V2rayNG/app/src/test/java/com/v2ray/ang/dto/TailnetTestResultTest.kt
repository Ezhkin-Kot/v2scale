package com.v2ray.ang.dto

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertFalse
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class TailnetTestResultTest {
    @Test
    fun parsesASuccessfulPeerPing() {
        val r = TailnetTestResult.parse("""{"ok":true,"state":"running","latency_ms":23,"path":"direct","peer":"ezhkin-drive"}""")
        assertTrue(r.ok)
        assertEquals(23, r.latencyMillis)
        assertEquals("direct", r.path)
        assertEquals("ezhkin-drive", r.peer)
        assertEquals("", r.error)
    }

    @Test
    fun parsesAFailureWithItsReason() {
        val r = TailnetTestResult.parse("""{"ok":false,"state":"running","path":"tcp","peer":"h:443","error":"i/o timeout"}""")
        assertFalse(r.ok)
        assertEquals("i/o timeout", r.error)
    }

    @Test
    fun missingFieldsFallBackToDefaults() {
        val r = TailnetTestResult.parse("{}")
        assertFalse(r.ok)
        assertEquals("", r.path)
    }

    @Test
    fun blankOrMalformedInputIsAFailureWithAReason() {
        assertEquals("no result", TailnetTestResult.parse(null).error)
        assertEquals("no result", TailnetTestResult.parse("").error)
        assertEquals("no result", TailnetTestResult.parse("{not json").error)
    }
}
