package com.v2ray.ang.handler

import org.junit.jupiter.api.Assertions.assertEquals
import org.junit.jupiter.api.Assertions.assertNotEquals
import org.junit.jupiter.api.Assertions.assertNull
import org.junit.jupiter.api.Assertions.assertTrue
import org.junit.jupiter.api.Test

class LocalProxyAuthTest {
    private val auto = "autoUser" to "autoPass"

    @Test
    fun userCredentialsAlwaysWin() {
        assertEquals("u" to "p", LocalProxyAuth.effective("u", "p", tailnetEnabled = true, auto = auto))
        assertEquals("u" to "p", LocalProxyAuth.effective("u", "p", tailnetEnabled = false, auto = null))
    }

    @Test
    fun generatedCredentialsApplyOnlyWhileTailnetIsOn() {
        assertEquals(auto, LocalProxyAuth.effective(null, null, tailnetEnabled = true, auto = auto))
        assertEquals(auto, LocalProxyAuth.effective("", "", tailnetEnabled = true, auto = auto))
        assertNull(LocalProxyAuth.effective(null, null, tailnetEnabled = false, auto = auto))
    }

    @Test
    fun halfSetUserCredentialsAreNotUsed() {
        assertEquals(auto, LocalProxyAuth.effective("u", "", tailnetEnabled = true, auto = auto))
        assertNull(LocalProxyAuth.effective("u", null, tailnetEnabled = false, auto = auto))
    }

    @Test
    fun withoutStoredCredentialsNothingIsInventedHere() {
        assertNull(LocalProxyAuth.effective(null, null, tailnetEnabled = true, auto = null))
    }

    @Test
    fun generatedCredentialsAreLongRandomAndSafeForTheHevConfig() {
        val (u1, p1) = LocalProxyAuth.generate()
        val (u2, p2) = LocalProxyAuth.generate()
        listOf(u1, p1, u2, p2).forEach {
            assertEquals(24, it.length)
            // The tunnel config quotes them in single quotes, so no quote, backslash or space may appear.
            assertTrue(it.all { c -> c.isLetterOrDigit() }, it)
        }
        assertNotEquals(u1, u2)
        assertNotEquals(p1, p2)
        assertNotEquals(u1, p1)
    }
}
