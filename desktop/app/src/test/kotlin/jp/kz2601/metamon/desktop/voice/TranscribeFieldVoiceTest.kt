package jp.kz2601.metamon.desktop.voice

import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.renderer.form.FormDraft
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Test

/** transcribeField のマイク: 作り物の Transcriber の結果が欄の後ろに足される(#1751)。 */
class TranscribeFieldVoiceTest {
    private class FakeMic : Microphone {
        override fun devices() = listOf("既定")
        override fun start(device: String?): Microphone.Recording = object : Microphone.Recording {
            override fun stop() = FloatArray(16_000) { (0.3 * kotlin.math.sin(2 * Math.PI * 440 * it / 16_000.0)).toFloat() }
            override fun cancel() {}
        }
    }

    private class FakeTranscriber(var text: String, override val isAvailable: Boolean = true) : Transcriber {
        override val pendingDownloadMegabytes: Int? = null
        override suspend fun transcribe(samples: FloatArray, language: String): String = text
    }

    private val form = FormDraft(Entity(id = "notes"))

    /** 押して録り、もう一度押して文字にし終えるまで待つ。欄への書き込みはその場で(画面のスレッドの代わり)。 */
    private fun VoiceInput.dictate(): String? {
        var posted: String? = null
        toggleInto(form, "memo", post = {}, onUi = { it() })
        toggleInto(form, "memo", post = { posted = it }, onUi = { it() })
        runBlocking { job?.join() }
        return posted
    }

    @Test
    fun heardTextIsAppendedToTheField() {
        val tr = FakeTranscriber("牛乳を買う")
        val v = VoiceInput(FakeMic(), tr, language = { "ja" })
        v.dictate()
        assertEquals(JSONValue.Str("牛乳を買う"), form.values["memo"])
        tr.text = "卵も"
        v.dictate()
        assertEquals(JSONValue.Str("牛乳を買う\n卵も"), form.values["memo"])
        assertEquals(VoiceInput.State.IDLE, v.state)
    }

    @Test
    fun nothingHeardLeavesTheField() {
        val v = VoiceInput(FakeMic(), FakeTranscriber("  "), language = { "ja" })
        v.dictate()
        assertNull(form.values["memo"]?.takeIf { it != JSONValue.Null })
    }

    @Test
    fun readOnlyFormIsNotWritten() {
        form.isReadOnly = true
        VoiceInput(FakeMic(), FakeTranscriber("書かない"), language = { "ja" }).dictate()
        assertNull(form.values["memo"]?.takeIf { it != JSONValue.Null })
    }

    @Test
    fun unavailableTranscriberHidesTheMic() {
        assertFalse(VoiceInput(FakeMic(), FakeTranscriber("x", isAvailable = false)).isAvailable)
    }

    @Test
    fun appendingFollowsIos() {
        assertEquals("新しい", DictationText.appending(" 新しい ", ""))
        assertEquals("前", DictationText.appending("  ", "前"))
        assertEquals("前\n新しい", DictationText.appending("新しい", "前"))
        assertEquals("前\n新しい", DictationText.appending("新しい", "前\n"))
        assertEquals("新しい", DictationText.appending("新しい", "  \n"))
    }
}
