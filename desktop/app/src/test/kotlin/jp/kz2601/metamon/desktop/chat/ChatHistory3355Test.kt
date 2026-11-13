package jp.kz2601.metamon.desktop.chat

import jp.kz2601.metamon.desktop.chat.ChatHistoryEntry.Kind
import jp.kz2601.metamon.desktop.i18n.Lang
import jp.kz2601.metamon.desktop.i18n.StringTable
import jp.kz2601.metamon.desktop.i18n.Strings
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

/** 会話の画面を開いた時の閉じた会話の読み直しと「最初から」(iOS の AppRuntime+ChatHistory・#3355)。 */
class ChatHistory3355Test {
    private fun texts(l: List<ChatHistoryEntry>) = l.map { it.text }

    @Test
    fun reopeningMakesEverythingClosedAndNewTalkFollows() {
        val port = ChatHistoryPort.InMemory()
        val s = ChatHistoryState(port, "app1")
        s.reload()
        s.append(Kind.USER, "家計簿")
        s.append(Kind.ASSISTANT, "作ります")
        assertEquals(listOf("家計簿", "作ります"), texts(port.load("app1")))

        val again = ChatHistoryState(port, "app1").also { it.reload() }
        assertEquals("前の会話は閉じた会話", listOf("家計簿", "作ります"), texts(again.history))
        assertTrue(again.live.isEmpty())
        again.append(Kind.USER, "色を変えて")
        assertEquals(listOf("色を変えて"), texts(again.live))
        assertEquals("端末には閉じた会話 + 今の会話", listOf("家計簿", "作ります", "色を変えて"), texts(port.load("app1")))
    }

    @Test
    fun appliedSeparatorClosesTheTalkOnlyWhenChatWasUsed() {
        val port = ChatHistoryPort.InMemory()
        val s = ChatHistoryState(port, "app1").also { it.reload() }
        s.noteApplied(2, "色を変えた")
        assertFalse("会話を使っていないアプリには区切りを入れない", s.hasHistory)
        s.append(Kind.USER, "色を変えて")
        s.noteApplied(3, "色を変えた")
        assertEquals(listOf(Kind.USER, Kind.APPLIED), s.history.map { it.kind })
        assertEquals(3, s.history.last().version)
        assertTrue(s.live.isEmpty())
    }

    @Test
    fun restartWithoutHistoryClearsOnlyTheLiveTalkWithoutAsking() {
        val port = ChatHistoryPort.InMemory()
        val s = ChatHistoryState(port, "app1").also { it.reload() }
        s.append(Kind.USER, "a")
        s.requestRestart()
        assertFalse("履歴が無ければ聞かない", s.restartConfirmOpen)
        assertTrue(s.live.isEmpty())
        assertTrue(port.load("app1").isEmpty())
    }

    @Test
    fun restartWithHistoryAsksThenLiveOnlyKeepsTheHistory() {
        val port = ChatHistoryPort.InMemory()
        port.save(listOf(ChatHistoryEntry(Kind.USER, "前"), ChatHistoryEntry(Kind.APPLIED, "", 1)), "app1")
        val s = ChatHistoryState(port, "app1").also { it.reload() }
        s.append(Kind.USER, "今")
        s.requestRestart()
        assertTrue(s.restartConfirmOpen)
        assertEquals("確認の間は何も消さない", listOf("今"), texts(s.live))
        s.restartLive()
        assertFalse(s.restartConfirmOpen)
        assertTrue(s.live.isEmpty())
        assertEquals(listOf("前", ""), texts(s.history))
        assertEquals("今の会話だけ消えて残る", listOf("前", ""), texts(port.load("app1")))
    }

    @Test
    fun restartAllClearsEverythingAndCancelKeepsIt() {
        val port = ChatHistoryPort.InMemory()
        port.save(listOf(ChatHistoryEntry(Kind.USER, "前")), "app1")
        port.save(listOf(ChatHistoryEntry(Kind.USER, "別")), "app2")
        val s = ChatHistoryState(port, "app1").also { it.reload() }
        s.append(Kind.USER, "今")
        s.requestRestart()
        s.dismissRestart()
        assertEquals(listOf("今"), texts(s.live))
        assertEquals(listOf("前"), texts(s.history))
        s.requestRestart()
        s.restartAll()
        assertFalse(s.hasHistory)
        assertTrue(s.live.isEmpty())
        assertTrue(port.load("app1").isEmpty())
        assertEquals("他のアプリの履歴は消さない", listOf("別"), texts(port.load("app2")))
    }

    @Test
    fun switchingAppsReadsThatAppsHistoryAndDropsTheConfirm() {
        val port = ChatHistoryPort.InMemory()
        port.save(listOf(ChatHistoryEntry(Kind.USER, "A")), "a")
        port.save(listOf(ChatHistoryEntry(Kind.USER, "B")), "b")
        val s = ChatHistoryState(port, "a").also { it.reload() }
        s.requestRestart()
        s.reload("b")
        assertFalse(s.restartConfirmOpen)
        assertEquals(listOf("B"), texts(s.history))
        assertEquals("b", s.appId)
    }

    @Test
    fun keepsTheLatest200() {
        val port = ChatHistoryPort.InMemory()
        val s = ChatHistoryState(port, "app1").also { it.reload() }
        repeat(205) { s.append(Kind.USER, "$it") }
        s.archiveLive()
        assertEquals(ChatHistoryState.LIMIT, s.history.size)
        assertEquals("5", s.history.first().text)
    }

    @Test
    fun textsUseTheAndroidKeys() {
        val t = StringTable.fromClasspath()
        for (key in listOf(
            "voice_history_applied", "voice_history_applied_plain", "voice_restart", "voice_restart_clear_all",
            "voice_restart_confirm_body", "voice_restart_confirm_title", "voice_restart_live_only", "inputs_cancel",
        )) {
            assertTrue("$key(ja)", t.hasString(Lang.JA, key))
            assertTrue("$key(en)", t.hasString(Lang.EN, key))
        }
        Strings.withLang(Lang.JA) {
            assertEquals("ここで反映しました(v3)・色を変えた", ChatHistoryText.applied(ChatHistoryEntry(Kind.APPLIED, " 色を変えた ", 3)))
            assertEquals("ここで反映しました", ChatHistoryText.applied(ChatHistoryEntry(Kind.APPLIED, "")))
        }
    }
}
