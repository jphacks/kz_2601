package jp.kz2601.metamon.desktop.io

import jp.kz2601.metamon.desktop.Examples
import jp.kz2601.metamon.desktop.MockStore
import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.desktop.voice.Transcriber
import jp.kz2601.metamon.dsl.Accept
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.dsl.records.FieldType
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

/** fileImport の録音ファイル: wav は文字起こしに渡る / 対応しない形式・off の時は案内(Transcriber は偽物)。 */
class AudioImportTest {
    private class FakeTranscriber(
        var text: String = "牛乳を買う",
        override val isAvailable: Boolean = true,
    ) : Transcriber {
        override val pendingDownloadMegabytes: Int? = null
        val calls = mutableListOf<Pair<Int, String>>()
        override suspend fun transcribe(samples: FloatArray, language: String): String {
            calls += samples.size to language
            return text
        }
    }

    private val samples = FloatArray(32_000) { 0.1f }
    private val fakeDecode: (File) -> FloatArray = { samples }

    private fun file(ext: String) = File.createTempFile("rec-", ".$ext").apply { writeBytes(ByteArray(8)); deleteOnExit() }

    private fun audio(t: Transcriber?) = AudioImport(t, { "ja" }, fakeDecode)

    @Test
    fun wavGoesToTranscriber() = runBlocking {
        val t = FakeTranscriber()
        assertEquals(AudioImport.Result.Text("牛乳を買う"), audio(t).transcribe(file("wav")))
        assertEquals(listOf(samples.size to "ja"), t.calls)
    }

    @Test
    fun otherFormatsAreRefusedWithoutTranscribing() = runBlocking {
        val t = FakeTranscriber()
        for (ext in listOf("m4a", "mp3")) {
            assertEquals(AudioImport.Result.Refused(Strings["desktop_io_audio_wav_only"]), audio(t).transcribe(file(ext)))
        }
        assertTrue(t.calls.isEmpty())
    }

    @Test
    fun offAndUnavailableAndSilentAreExplained() = runBlocking {
        assertEquals(AudioImport.Result.Refused(Strings["desktop_io_audio_off"]), audio(null).transcribe(file("wav")))
        assertEquals(AudioImport.Result.Refused(Strings["desktop_voice_unavailable"]), audio(FakeTranscriber(isAvailable = false)).transcribe(file("wav")))
        assertEquals(AudioImport.Result.Refused(Strings["voice_b_not_heard"]), audio(FakeTranscriber(text = " ")).transcribe(file("wav")))
    }

    @Test
    fun brokenWavIsUnreadable() = runBlocking {
        val broken = AudioImport(FakeTranscriber(), { "ja" }, { error("bad") })
        assertEquals(AudioImport.Result.Refused(Strings["desktop_io_audio_unreadable"]), broken.transcribe(file("wav")))
    }

    @Test
    fun importedTextBecomesOneRecordAndRefusalBecomesNotice() {
        var store = MockStore()
        val dsl = Examples.load("shopping")
        val probe = DesktopIo(dsl, { store }, { store = it })
        val entity = probe.entities.first { e -> e.storedFields.any { it.type == FieldType.STRING } }
        val field = entity.storedFields.first { it.type == FieldType.STRING }.id
        val c = Component(type = ComponentType.FILE_IMPORT, entity = entity.id, field = field, accept = listOf(Accept.AUDIO))
        val direct: (() -> Unit) -> Unit = { it() }

        val ok = DesktopIo(dsl, { store }, { store = it }, audio(FakeTranscriber()), post = direct)
        runBlocking { ok.importAudio(file("wav"), entity, c).join() }
        assertEquals(Strings["desktop_io_imported"], ok.notice)
        assertEquals(1, store.list(entity.id).size)

        val off = DesktopIo(dsl, { store }, { store = it }, audio(null), post = direct)
        off.importFile(file("wav"), c)
        val m4a = DesktopIo(dsl, { store }, { store = it }, audio(FakeTranscriber()), post = direct)
        m4a.importFile(file("m4a"), c)
        awaitNotice(m4a, Strings["desktop_io_audio_wav_only"])
        awaitNotice(off, Strings["desktop_io_audio_off"])
    }

    private fun awaitNotice(io: DesktopIo, expected: String) {
        repeat(200) { if (io.notice == expected) return else Thread.sleep(10) }
        assertEquals(expected, io.notice)
    }
}
