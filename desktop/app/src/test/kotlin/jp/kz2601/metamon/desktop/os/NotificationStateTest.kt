package jp.kz2601.metamon.desktop.os

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class NotificationStateTest {
    @Test
    fun qunsTable() {
        val expected = mapOf(
            1 to false, // 席を外している
            2 to true, // 応答不可
            3 to true, // 全画面(D3D)
            4 to true, // プレゼン中
            5 to false, // 受け付ける
            6 to true, // 集中(quiet time)
            7 to true, // 全画面のアプリ
            0 to false,
            8 to false,
            -1 to false,
        )
        expected.forEach { (v, quiet) -> assertEquals("QUNS $v", quiet, WindowsQuietState.isQuiet(v)) }
    }

    @Test
    fun unreadableMeansShow() {
        assertFalse(WindowsQuietState.isQuiet(null))
        assertFalse(WindowsQuietState.reader { null }.isQuiet())
        assertTrue(WindowsQuietState.reader { 4 }.isQuiet())
    }

    @Test
    fun readerThatThrowsMeansShow() {
        assertFalse(NotificationState.shouldHold(true) { error("boom") })
    }

    @Test
    fun offByDefaultAndNeverReads() {
        assertFalse(NotificationState.enabled(null))
        assertFalse(NotificationState.enabled(""))
        assertFalse(NotificationState.enabled("0"))
        var read = false
        assertFalse(NotificationState.shouldHold(false) { read = true; true })
        assertFalse(read)
    }

    @Test
    fun onWhenSet() {
        listOf("1", "true", " ON ").forEach { assertTrue(it, NotificationState.enabled(it)) }
        assertTrue(NotificationState.shouldHold(true) { true })
        assertFalse(NotificationState.shouldHold(true) { false })
    }

    @Test
    fun notWindowsDefaultReaderShows() {
        if (!System.getProperty("os.name").orEmpty().startsWith("Windows", ignoreCase = true)) {
            assertFalse(WindowsQuietState.reader().isQuiet())
        }
    }
}
