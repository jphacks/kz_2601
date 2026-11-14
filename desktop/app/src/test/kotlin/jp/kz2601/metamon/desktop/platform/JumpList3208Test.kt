package jp.kz2601.metamon.desktop.platform

import jp.kz2601.metamon.desktop.HostOs
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertSame
import org.junit.Assert.assertTrue
import org.junit.Test

/** ジャンプリストの並びと引数(#3208)。OS に出す所は Windows でしか動かないので見ない。 */
class JumpList3208Test {
    private val labels = JumpListPlan.Labels("最近使ったアプリ", "すぐ記録", "話しかける")
    private val names = mapOf("shopping" to "買い物", "habit" to "習慣", "books" to "本", "diary" to "日記", "money" to "家計簿", "todo" to "やること")

    @Test
    fun recentKeepsNewestFirstWithoutDuplicatesUpToFive() {
        var r = emptyList<String>()
        for (id in listOf("a", "b", "c", "a", "d", "e", "f")) r = RecentApps.touch(r, id)
        assertEquals(listOf("f", "e", "d", "a", "c"), r)
        assertEquals(r, RecentApps.decode(RecentApps.encode(r)))
        assertEquals(emptyList<String>(), RecentApps.decode(null))
        assertEquals(listOf("x", "y"), RecentApps.decode("x\n\n y \nx"))
    }

    @Test
    fun planListsKnownRecentAppsThenTasks() {
        val c = JumpListPlan.build(listOf("todo", "gone", "shopping", "habit", "books", "diary", "money"), names, canQuickRecord = true, voice = false, labels)
        assertEquals("最近使ったアプリ", c.categoryTitle)
        assertEquals(listOf("やること", "買い物", "習慣", "本", "日記"), c.recent.map { it.title })
        assertEquals(listOf("todo"), c.recent.first().args)
        assertEquals(listOf("すぐ記録"), c.tasks.map { it.title })
        assertEquals(listOf("--jump=quick-record"), c.tasks.single().args)
    }

    @Test
    fun voiceOnlyWhenEnabledAndQuickRecordOnlyWhenWritable() {
        val c = JumpListPlan.build(emptyList(), names, canQuickRecord = false, voice = true, labels)
        assertEquals(listOf("話しかける"), c.tasks.map { it.title })
        assertEquals(listOf("--jump=voice"), c.tasks.single().args)
        assertTrue(JumpListPlan.build(emptyList(), names, canQuickRecord = false, voice = false, labels).isEmpty)
    }

    @Test
    fun jumpArgsRoundTripAndKeepOtherArgsInOrder() {
        val argv = listOf("--jump=quick-record", "shopping")
        assertEquals(JumpCommand.QUICK_RECORD, JumpArgs.parse(argv))
        assertEquals(listOf("shopping"), JumpArgs.strip(argv))
        assertEquals(JumpCommand.VOICE, JumpArgs.parse(JumpArgs.encode(JumpCommand.VOICE)))
        assertNull(JumpArgs.parse(listOf("shopping")))
        assertNull("知らない指示は無視", JumpArgs.parse(listOf("--jump=format-disk")))
        assertEquals(listOf("shopping", "rec.json", "home"), JumpArgs.strip(listOf("shopping", "--jump=voice", "rec.json", "home")))
    }

    @Test
    fun commandLineQuotesLikeWindows() {
        assertEquals("shopping", JumpListPlan.commandLine(listOf("shopping")))
        assertEquals("--jump=quick-record", JumpListPlan.commandLine(JumpArgs.encode(JumpCommand.QUICK_RECORD)))
        assertEquals("\"a b\" \"\"", JumpListPlan.commandLine(listOf("a b", "")))
        assertEquals("\"say \\\"hi\\\"\"", JumpListPlan.commandLine(listOf("say \"hi\"")))
        assertEquals("\"C:\\dir x\\\\\"", JumpListPlan.commandLine(listOf("C:\\dir x\\")))
    }

    @Test
    fun offByDefaultAndOnlyOnWindows() {
        val none: (String) -> String? = { null }
        val on: (String) -> String? = { if (it == "TAILORIT_JUMPLIST") "1" else null }
        assertFalse(JumpListPublisher.enabled(HostOs.WINDOWS, none, none))
        assertTrue(JumpListPublisher.enabled(HostOs.WINDOWS, none, on))
        assertFalse(JumpListPublisher.enabled(HostOs.MAC, none, on))
        assertFalse(JumpListPublisher.enabled(HostOs.LINUX, none, on))
        assertSame(JumpListPublisher.Noop, JumpListPublisher.create(HostOs.MAC, enabled = false))
        assertFalse(JumpListPublisher.Noop.publish(JumpListPlan.build(listOf("todo"), names, true, true, labels)))
    }

    @Test
    fun guidBytesAreLittleEndianLikeWindows() {
        // {00021401-0000-0000-C000-000000000046}
        val b = WindowsJumpList.guidBytes(WindowsJumpList.CLSID_SHELL_LINK)
        assertEquals(listOf(0x01, 0x14, 0x02, 0x00, 0, 0, 0, 0, 0xC0, 0, 0, 0, 0, 0, 0, 0x46), b.map { it.toInt() and 0xff })
    }
}
