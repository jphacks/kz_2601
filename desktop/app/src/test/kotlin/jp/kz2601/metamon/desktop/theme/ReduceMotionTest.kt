package jp.kz2601.metamon.desktop.theme

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class ReduceMotionTest {
    @Test
    fun overrideFromEnv() {
        listOf("1", "true", " ON ").forEach { assertEquals(it, true, ReduceMotion.parseOverride(it)) }
        listOf("0", "false", "off").forEach { assertEquals(it, false, ReduceMotion.parseOverride(it)) }
        listOf(null, "", "maybe").forEach { assertNull(it, ReduceMotion.parseOverride(it)) }
    }

    @Test
    fun overrideWinsOverOs() {
        assertTrue(ReduceMotion.resolve(true, false))
        assertFalse(ReduceMotion.resolve(false, true))
        assertTrue(ReduceMotion.resolve(null, true))
    }

    @Test
    fun unreadableDefaultsToNotReducing() {
        assertFalse(ReduceMotion.resolve(null, null))
        assertNull(ReduceMotion.detect("linux", run = { null }))
        assertNull(ReduceMotion.detect("linux", run = { error("no gsettings") }))
        assertNull(ReduceMotion.detect("windows 11", windows = { null }))
        assertNull(ReduceMotion.detect("windows 11", windows = { error("jna") }))
        assertNull(ReduceMotion.detect("mac os x"))
    }

    @Test
    fun gsettingsEnableAnimations() {
        assertEquals(false, ReduceMotion.parseGsettings("true\n"))
        assertEquals(true, ReduceMotion.parseGsettings("false\n"))
        assertNull(ReduceMotion.parseGsettings("garbage"))
        assertEquals(true, ReduceMotion.detect("linux", run = { "false\n" }))
        assertEquals(false, ReduceMotion.detect("linux", run = { "true\n" }))
    }

    @Test
    fun windowsClientAreaAnimation() {
        assertEquals(true, ReduceMotion.fromClientAreaAnimation(true, 0))
        assertEquals(false, ReduceMotion.fromClientAreaAnimation(true, 1))
        assertNull(ReduceMotion.fromClientAreaAnimation(false, 0))
        assertEquals(true, ReduceMotion.detect("windows 11", windows = { true }))
    }

    @Test
    fun currentSkipsOsWhenOverridden() {
        assertTrue(ReduceMotion.current(true))
        assertFalse(ReduceMotion.current(false))
    }
}
