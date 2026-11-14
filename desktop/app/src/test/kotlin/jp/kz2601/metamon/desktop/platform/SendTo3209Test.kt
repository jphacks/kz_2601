package jp.kz2601.metamon.desktop.platform

import jp.kz2601.metamon.desktop.Examples
import jp.kz2601.metamon.desktop.HostOs
import jp.kz2601.metamon.desktop.MockStore
import jp.kz2601.metamon.desktop.io.DesktopIo
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentType
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertSame
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder
import java.io.File

/** Windows の「送る」(#3209): 引数のファイルをドラッグ & ドロップと同じ振り分けに渡す。 */
class SendTo3209Test {
    @get:Rule
    val tmp = TemporaryFolder()

    @Test
    fun filesAfterFlagAndOtherArgsKept() {
        val argv = listOf("--send-to", "C:\\a b\\x.csv", "C:\\y.pdf")
        assertEquals(listOf(File("C:\\a b\\x.csv"), File("C:\\y.pdf")), SendToArgs.files(argv))
        assertEquals(emptyList<String>(), SendToArgs.strip(argv))
        assertEquals(listOf("shopping"), SendToArgs.strip(listOf("shopping", "--send-to", "x.csv")))
        assertNull(SendToArgs.files(listOf("shopping", "rec.json", "home")))
        assertEquals(listOf("shopping", "rec.json", "home"), SendToArgs.strip(listOf("shopping", "rec.json", "home")))
        assertEquals(emptyList<File>(), SendToArgs.files(listOf("--send-to")))
    }

    @Test
    fun onlyReadableFilesOnce() {
        val csv = tmp.newFile("a.csv")
        val dir = tmp.newFolder("folder")
        val gone = File(tmp.root, "gone.csv")
        assertEquals(listOf(csv), SendToArgs.readable(listOf(csv, dir, gone, File(csv.path))))
    }

    @Test
    fun inboxDeliversOnceToTargetWindow() {
        val box = SendToInbox()
        val f = listOf(File("a.csv"))
        box.offer(emptyList(), 1)
        assertEquals(0, box.requests)
        box.offer(f, 2)
        assertEquals(1, box.requests)
        assertEquals("ほかのウィンドウは受け取らない", emptyList<File>(), box.take(1) { true })
        assertEquals(f, box.take(2) { false })
        assertEquals("1 回だけ", emptyList<File>(), box.take(2) { true })
        // 行き先が無ければ前にあるウィンドウ
        box.offer(f, null)
        assertEquals(emptyList<File>(), box.take(3) { false })
        assertEquals(f, box.take(3) { true })
    }

    @Test
    fun sentCsvGoesToTheSameImportAsADrop() {
        val csv = tmp.newFile("list.csv").apply { writeText("name,qty\n牛乳,2\n卵,1\n") }
        val io = DesktopIo(Examples.load("shopping"), { MockStore() }, {})
        val screen = listOf(Component(type = ComponentType.LIST, entity = "groceries"))
        val box = SendToInbox().apply { offer(SendToArgs.readable(SendToArgs.files(listOf("--send-to", csv.path))!!), 1) }
        assertTrue(io.dropFiles(box.take(1) { true }, screen, null))
        assertNotNull("取り込みの確認を出す(落とした時と同じ)", io.pending)
        assertEquals("groceries", io.pending!!.entityId)
    }

    @Test
    fun sentVideoFillsTheFormLikeADrop() {
        val clip = tmp.newFile("clip.mp4")
        val io = DesktopIo(Examples.load("shopping"), { MockStore() }, {})
        val draft = mutableMapOf<String, String>()
        assertTrue(io.dropFiles(SendToArgs.files(listOf("--send-to", clip.path))!!, listOf(Component(type = ComponentType.VIDEO_FIELD, field = "clip")), draft))
        assertEquals(clip.toURI().toString(), draft["clip"])
    }

    @Test
    fun shortcutOnlyOnInstalledWindows() {
        assertSame(SendToShortcut.Noop, SendToShortcut.create(HostOs.MAC, "/Applications/Tailor It.app", "/x"))
        assertSame(SendToShortcut.Noop, SendToShortcut.create(HostOs.WINDOWS, null, "C:\\Users\\a\\AppData\\Roaming"))
        assertSame(SendToShortcut.Noop, SendToShortcut.create(HostOs.WINDOWS, "C:\\Tailor It\\Tailor It.exe", null))
        assertEquals("Tailor It.lnk", SendToShortcut.linkFile("/roaming")!!.name)
        assertTrue(SendToShortcut.linkFile("/roaming")!!.path.contains("SendTo"))
        assertEquals(
            listOf(0x0b, 0x01, 0, 0, 0, 0, 0, 0, 0xC0, 0, 0, 0, 0, 0, 0, 0x46),
            WindowsSendToShortcut.guidBytes(WindowsSendToShortcut.IID_PERSIST_FILE).map { it.toInt() and 0xff },
        )
    }
}
