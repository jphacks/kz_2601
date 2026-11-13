package jp.kz2601.metamon.desktop

import androidx.compose.ui.input.key.Key
import androidx.compose.ui.text.TextRange
import androidx.compose.ui.text.input.TextFieldValue
import jp.kz2601.metamon.desktop.chat.ChatErrorCopy
import jp.kz2601.metamon.desktop.chat.ChatKey
import jp.kz2601.metamon.desktop.chat.ChatSession
import jp.kz2601.metamon.desktop.chat.insertNewline
import jp.kz2601.metamon.desktop.i18n.Lang
import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.services.AIClient
import jp.kz2601.metamon.services.ChatMessage
import jp.kz2601.metamon.services.ChatMode
import jp.kz2601.metamon.services.ChatTurn
import jp.kz2601.metamon.services.GenerateMode
import jp.kz2601.metamon.services.GenerateProgress
import jp.kz2601.metamon.services.MorphResult
import jp.kz2601.metamon.services.http.FailureKind
import jp.kz2601.metamon.services.http.ProRequired
import jp.kz2601.metamon.services.http.RateLimitInfo
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.time.Instant

/** 会話の画面(#2381): Enter / Shift+Enter / Esc・F1 の一覧・パレットの「新しいアプリを作る」・失敗の文言のキー。 */
class ChatKeys2381Test {
    private object NoAi : AIClient {
        override suspend fun morph(transcript: String, appId: String, currentVersion: Int, locale: String?, mode: GenerateMode): MorphResult = error("unused")
        override suspend fun morphStreaming(
            transcript: String,
            appId: String,
            currentVersion: Int,
            locale: String?,
            mode: GenerateMode,
            progress: (GenerateProgress) -> Unit,
        ): MorphResult = error("unused")
        override suspend fun chat(appId: String, messages: List<ChatMessage>, locale: String?, mode: ChatMode?): ChatTurn = error("unused")
        override suspend fun resetChat(appId: String) = Unit
    }

    @Test
    fun enterSendsShiftEnterBreaksEscCloses() {
        assertEquals(ChatKey.SEND, ChatKey.of(KeyPress(Key.Enter)))
        assertEquals(ChatKey.SEND, ChatKey.of(KeyPress(Key.NumPadEnter)))
        assertEquals(ChatKey.NEWLINE, ChatKey.of(KeyPress(Key.Enter, shift = true)))
        assertEquals(ChatKey.CLOSE, ChatKey.of(KeyPress(Key.Escape)))
        // Ctrl / ⌘ / Alt と組んだ物・他のキーは入力欄に任せる
        assertNull(ChatKey.of(KeyPress(Key.Enter, ctrl = true)))
        assertNull(ChatKey.of(KeyPress(Key.Enter, meta = true)))
        assertNull(ChatKey.of(KeyPress(Key.Enter, alt = true)))
        assertNull(ChatKey.of(KeyPress(Key.A)))
    }

    @Test
    fun helpListsChatKeysPerOs() {
        Strings.withLang(Lang.JA) {
            assertEquals(listOf("送る" to "Enter", "改行" to "Shift+Enter", "会話を閉じる" to "Esc"), ChatKey.help(HostOs.WINDOWS))
            assertEquals(listOf("↩", "⇧↩", "Esc"), ChatKey.help(HostOs.MAC).map { it.second })
        }
        Strings.withLang(Lang.EN) {
            assertEquals(listOf("Send", "New line", "Close the chat"), ChatKey.help(HostOs.LINUX).map { it.first })
        }
    }

    @Test
    fun shiftEnterReplacesSelectionWithNewline() {
        assertEquals(TextFieldValue("ab\ncd", TextRange(3)), insertNewline(TextFieldValue("abcd", TextRange(2))))
        assertEquals(TextFieldValue("a\nd", TextRange(2)), insertNewline(TextFieldValue("abcd", TextRange(3, 1))))
    }

    @Test
    fun paletteOffersNewAppOnlyWithAiAndOpensChat() {
        val s = DesktopSession(Examples.load("shopping"), MockStore())
        val k = DesktopKeyboard(s.dsl, s, HostOs.WINDOWS)
        assertTrue(k.commands().none { it.id == "chat:new" })
        assertFalse(k.openChat())

        k.newChat = { ChatSession(NoAi, appId = "new") }
        val cmd = Strings.withLang(Lang.JA) { k.commands().first { it.id == "chat:new" } }
        assertEquals("新しいアプリを作る", cmd.title)
        cmd.run()
        assertNotNull(k.chat)
        // 会話を開いている間はショートカットを取らない(Enter・Esc は入力欄が受ける)
        assertFalse(k.handle(KeyPress(Key.N, ctrl = true), bubble = false))
        assertFalse(k.handle(KeyPress(Key.Escape), bubble = true))
    }

    @Test
    fun errorCopyUsesAndroidKeys() {
        val pro = ChatErrorCopy.of(FailureKind.ProRequiredKind(ProRequired(emptyList())))
        assertEquals("voice_plan_limit_title" to "voice_pro_required_body", pro.titleKey to pro.bodyKey)
        assertFalse(pro.retry)
        assertTrue(ChatErrorCopy.of(FailureKind.Busy(null)).retry)
        assertEquals("voice_busy_title", ChatErrorCopy.of(FailureKind.Busy(1.0)).titleKey)
        val rate = ChatErrorCopy.of(FailureKind.RateLimited(RateLimitInfo(RateLimitInfo.Scope.USER, resetAt = Instant.parse("2026-01-02T03:04:00Z"))))
        assertEquals("voice_rate_title", rate.titleKey)
        assertFalse(rate.retry)
        Strings.withLang(Lang.JA) {
            assertTrue(rate.body(java.time.ZoneOffset.UTC), rate.body(java.time.ZoneOffset.UTC).contains("3:04"))
            val noReset = ChatErrorCopy.of(FailureKind.RateLimited(RateLimitInfo(RateLimitInfo.Scope.USER)))
            assertTrue(noReset.body().contains(Strings["voice_tomorrow"]))
        }
        // 使うキーは日英どちらの表にもある
        val kinds = listOf(
            FailureKind.Offline, FailureKind.Timeout, FailureKind.Busy(null), FailureKind.SessionExpired, FailureKind.AppCheckFailed,
            FailureKind.AIPaused, FailureKind.FreePlanPaused(null), FailureKind.ProRequiredKind(ProRequired(emptyList())), FailureKind.AppStopped,
            FailureKind.RateLimited(RateLimitInfo(RateLimitInfo.Scope.USER)), FailureKind.Conflict(null), FailureKind.Rejected("x"), FailureKind.Other("x"),
            FailureKind.Cancelled,
        )
        for (k in kinds) {
            val c = ChatErrorCopy.of(k)
            for (lang in listOf(Lang.JA, Lang.EN)) {
                assertTrue("${c.titleKey} $lang", Strings.table.hasString(lang, c.titleKey))
                assertTrue("${c.bodyKey} $lang", Strings.table.hasString(lang, c.bodyKey))
            }
        }
    }
}
