package jp.kz2601.metamon.renderer

import jp.kz2601.metamon.dsl.Accept
import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.decode
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.entityModels
import jp.kz2601.metamon.dsl.screen
import jp.kz2601.metamon.renderer.components.importing.AudioFileImport
import jp.kz2601.metamon.renderer.components.importing.FileImportFlow
import jp.kz2601.metamon.renderer.components.importing.ImportKind
import jp.kz2601.metamon.renderer.components.importing.ImportMessage
import jp.kz2601.metamon.renderer.components.importing.ImportStep
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** fileImport の録音ファイル(#1180): 上限の下見・種類の見分け・確認の文(iOS の AudioFileImport と同じ)。 */
class AudioFileImportTest {
    @Test
    fun limitsMatchIosAndBackend() {
        assertEquals(1800.0, AudioFileImport.MAX_SECONDS, 0.0)
        assertEquals(200_000_000L, AudioFileImport.MAX_BYTES)
    }

    @Test
    fun checkOrdersProblemsLikeIos() {
        assertNull(AudioFileImport.check(bytes = 1_000, seconds = 60.0))
        assertNull("大きさが分からなくても長さで決める", AudioFileImport.check(bytes = null, seconds = 1800.0))
        assertEquals(AudioFileImport.Problem.TOO_LARGE, AudioFileImport.check(200_000_001, seconds = null))
        assertEquals(AudioFileImport.Problem.UNREADABLE, AudioFileImport.check(1_000, seconds = null))
        assertEquals(AudioFileImport.Problem.UNREADABLE, AudioFileImport.check(1_000, Double.NaN))
        assertEquals(AudioFileImport.Problem.EMPTY, AudioFileImport.check(1_000, 0.0))
        assertEquals(AudioFileImport.Problem.TOO_LONG, AudioFileImport.check(1_000, 1800.5))
    }

    @Test
    fun durationIsSplitIntoMinutesAndSeconds() {
        assertEquals(12 to 30, AudioFileImport.minutesAndSeconds(750.0))
        assertEquals(0 to 45, AudioFileImport.minutesAndSeconds(44.6))
        assertEquals(0 to 0, AudioFileImport.minutesAndSeconds(-3.0))
    }

    @Test
    fun audioIsRecognisedByMimeOrByExtensionWhenMimeIsVague() {
        assertTrue(AudioFileImport.isAudio("rec", "audio/mpeg"))
        assertTrue(AudioFileImport.isAudio("会議.M4A", "application/octet-stream"))
        assertTrue(AudioFileImport.isAudio("voice.ogg", null))
        assertFalse(AudioFileImport.isAudio("voice.ogg", "text/plain"))
        assertFalse(AudioFileImport.isAudio("memo.txt", null))
        assertEquals(ImportKind.AUDIO, FileImportFlow.kind("rec.wav", null, listOf(Accept.AUDIO)))
        assertNull(FileImportFlow.kind("rec.wav", "audio/wav", listOf(Accept.TEXT)))
    }

    @Test
    fun transcriptBecomesOneBodyRecordWithTheLengthNote() {
        val dsl = AppDSL.decode(
            """
            {"schemaVersion":1,"app":{"name":"t","voiceEntry":"none"},"initialScreen":"s",
             "entities":[{"id":"note","name":"メモ","titleField":"title","fields":[
               {"id":"title","name":"題","type":"string"},{"id":"body","name":"本文","type":"text"}]}],
             "screens":[{"id":"s","title":"s","components":[
               {"type":"fileImport","entity":"note","accept":["audio"],"field":"body"}]}]}
            """.trimIndent(),
        )
        val c: Component = dsl.screen("s")!!.components.single()
        val note = dsl.entityModels().single()
        val step = FileImportFlow.planText("今日の会議\n次は金曜", "会議.m4a", c, note, 0, note = "録音の長さ: 1 分 5 秒")
        step as ImportStep.Confirm
        assertEquals("今日の会議\n次は金曜", (step.rows.single()["body"] as JSONValue.Str).value)
        assertEquals(ImportMessage.Body("会議.m4a", "メモ", 10, false, "録音の長さ: 1 分 5 秒"), step.message)
    }
}
