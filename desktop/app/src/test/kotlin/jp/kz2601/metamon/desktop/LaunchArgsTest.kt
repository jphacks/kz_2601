package jp.kz2601.metamon.desktop

import jp.kz2601.metamon.desktop.i18n.Strings
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class LaunchArgsTest {
    @Test
    fun quickRecordFlagIsTakenOut() {
        val p = LaunchArgs.parse(listOf("--quick-record"))
        assertEquals(LaunchAction.QUICK_RECORD, p.action)
        assertEquals(emptyList<String>(), p.positional)
    }

    @Test
    fun flagAndPositionalCanMix() {
        val p = LaunchArgs.parse(listOf("shopping", "--new-window"))
        assertEquals(LaunchAction.NEW_WINDOW, p.action)
        assertEquals(listOf("shopping"), p.positional)
        assertEquals(LaunchAction.TALK, LaunchArgs.parse(listOf("--talk", "shopping")).action)
    }

    @Test
    fun noFlagKeepsArgsAsBefore() {
        val p = LaunchArgs.parse(listOf("shopping", "case.json", "home"))
        assertNull(p.action)
        assertEquals(listOf("shopping", "case.json", "home"), p.positional)
    }

    @Test
    fun unknownFlagIsDroppedAndFirstKnownFlagWins() {
        val p = LaunchArgs.parse(listOf("--nope", "--talk", "--quick-record", "x"))
        assertEquals(LaunchAction.TALK, p.action)
        assertEquals(listOf("x"), p.positional)
    }

    @Test
    fun afterDoubleDashEverythingIsPositional() {
        val p = LaunchArgs.parse(listOf("--", "--quick-record"))
        assertNull(p.action)
        assertEquals(listOf("--quick-record"), p.positional)
    }

    @Test
    fun desktopActionSectionsFollowTheSpec() {
        val text = LaunchArgs.desktopActionSections({ Strings[it.stringKey] })
        assertEquals("Actions=QuickRecord;NewWindow;Talk;", LaunchArgs.actionsLine())
        LaunchAction.entries.forEach { a ->
            assertTrue(text.contains("[Desktop Action ${a.desktopId}]"))
            assertTrue(text.contains("Exec=tailor-it ${a.flag}"))
        }
        assertTrue(text.contains("Name=すぐ記録"))
        // Actions= に並べた名前と節の名前が一致する
        val ids = Regex("""\[Desktop Action (\w+)]""").findAll(text).map { it.groupValues[1] }.toList()
        assertEquals(LaunchAction.entries.map { it.desktopId }, ids)
    }
}
