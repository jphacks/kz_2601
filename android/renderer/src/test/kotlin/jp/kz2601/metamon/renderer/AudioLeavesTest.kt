package jp.kz2601.metamon.renderer

import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.renderer.components.audio.AudioClaims
import jp.kz2601.metamon.renderer.components.audio.AudioRecorderMachine
import jp.kz2601.metamon.renderer.components.audio.AudioRecorderMachine.Event
import jp.kz2601.metamon.renderer.components.audio.AudioRecorderMachine.Phase
import jp.kz2601.metamon.renderer.components.audio.AudioRecorderMachine.RemoteAction
import jp.kz2601.metamon.renderer.components.audio.AudioRecorderMachine.RemoteCommand
import jp.kz2601.metamon.renderer.components.audio.AudioRecorderPolicy
import jp.kz2601.metamon.renderer.components.audio.AudioValue
import jp.kz2601.metamon.renderer.components.audio.DictationText
import jp.kz2601.metamon.renderer.components.audio.LocalAudioStore
import jp.kz2601.metamon.renderer.components.audio.SpeechVoice
import jp.kz2601.metamon.renderer.registry.ComponentRegistry
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.nio.file.Files

/** 音の部品(#864)の決まりごと。値の形・上限・状態遷移は iOS の LocalAudio.swift と同じ。 */
class AudioLeavesTest {
    @Test
    fun registersAllThree() {
        val types = ComponentRegistry.standard.registeredTypes
        listOf(ComponentType.AUDIO_RECORDER, ComponentType.SPEAK_BUTTON, ComponentType.TRANSCRIBE_FIELD).forEach {
            assertTrue(it.wire, it.wire in types)
        }
    }

    @Test
    fun audioValueRoundTripsLikeIos() {
        val v = AudioValue("ABC-123_x", 12)
        assertEquals("audio:ABC-123_x|12", v.encoded)
        assertEquals(v, AudioValue.parse(v.encoded))
        assertEquals(v, AudioValue.parse(JSONValue.Str(v.encoded)))
        assertEquals(AudioValue("abc", 0), AudioValue.parse("audio:abc"))
        assertEquals(AudioValue("abc", 0), AudioValue.parse("audio:abc|x"))
    }

    @Test
    fun audioValueRejectsPathsAndOtherValues() {
        assertNull(AudioValue.parse("audio:../etc|3"))
        assertNull(AudioValue.parse("audio:a/b|3"))
        assertNull(AudioValue.parse("audio:|3"))
        assertNull(AudioValue.parse("audio:" + "a".repeat(81)))
        assertNull(AudioValue.parse("video:abc|3"))
        assertNull(AudioValue.parse(JSONValue.Num(3.0)))
        assertNull(AudioValue.parse(null as JSONValue?))
        assertTrue(AudioValue.isValidId(AudioValue.newId()))
    }

    @Test
    fun localIdsInRecord() {
        val data = mapOf("a" to JSONValue.Str("audio:x1|3"), "b" to JSONValue.Str("hello"), "c" to JSONValue.Str("audio:y2|0"))
        assertEquals(setOf("x1", "y2"), AudioValue.localIds(data))
    }

    @Test
    fun maxDurationDefaultsAndClamps() {
        assertEquals(600, AudioRecorderPolicy.maxDuration(null))
        assertEquals(600, AudioRecorderPolicy.maxDuration(0))
        assertEquals(600, AudioRecorderPolicy.maxDuration(-5))
        assertEquals(30, AudioRecorderPolicy.maxDuration(30))
        assertEquals(7200, AudioRecorderPolicy.maxDuration(99_999))
    }

    @Test
    fun elapsedTextAndStoredSeconds() {
        assertEquals("0:00", AudioRecorderPolicy.elapsedText(0.0))
        assertEquals("0:05", AudioRecorderPolicy.elapsedText(5.9))
        assertEquals("10:00", AudioRecorderPolicy.elapsedText(600.0))
        assertEquals("1:00:01", AudioRecorderPolicy.elapsedText(3601.0))
        assertEquals("0:00", AudioRecorderPolicy.elapsedText(Double.NaN))
        assertEquals(1, AudioRecorderPolicy.storedSeconds(0.4))
        assertEquals(3, AudioRecorderPolicy.storedSeconds(3.0))
        assertEquals(0, AudioRecorderPolicy.storedSeconds(0.0))
        assertEquals(0, AudioRecorderPolicy.storedSeconds(Double.POSITIVE_INFINITY))
    }

    @Test
    fun meterLevel() {
        assertEquals(0.0, AudioRecorderPolicy.meterLevel(-60.0), 1e-9)
        assertEquals(0.5, AudioRecorderPolicy.meterLevel(-25.0), 1e-9)
        assertEquals(1.0, AudioRecorderPolicy.meterLevel(0.0), 1e-9)
        assertEquals(0.0, AudioRecorderPolicy.meterLevel(AudioRecorderPolicy.amplitudeToPower(0)), 1e-9)
        assertEquals(1.0, AudioRecorderPolicy.meterLevel(AudioRecorderPolicy.amplitudeToPower(32767)), 1e-9)
    }

    @Test
    fun machineTransitions() {
        val m = AudioRecorderMachine(hasClip = false)
        assertEquals(Phase.IDLE, m.phase)
        assertFalse(m.send(Event.STOP))
        assertTrue(m.send(Event.START))
        assertFalse(m.send(Event.START))
        assertTrue(m.send(Event.FAIL))
        assertEquals(Phase.IDLE, m.phase) // 録音が無かったので録音前に戻る
        m.send(Event.START)
        m.send(Event.STOP)
        assertEquals(Phase.RECORDED, m.phase)
        m.send(Event.START)
        m.send(Event.FAIL)
        assertEquals(Phase.RECORDED, m.phase) // 録り直しの失敗は元の録音に戻る
        assertTrue(m.send(Event.DISCARD))
        assertEquals(Phase.IDLE, m.phase)
    }

    @Test
    fun earphoneButtons() {
        val idle = AudioRecorderMachine(hasClip = false)
        assertEquals(RemoteAction.START_RECORDING, idle.remoteAction(RemoteCommand.TOGGLE_PLAY_PAUSE, isPlaying = false))
        assertEquals(RemoteAction.START_RECORDING, idle.remoteAction(RemoteCommand.PLAY, isPlaying = false))
        assertNull(idle.remoteAction(RemoteCommand.PAUSE, isPlaying = false))
        val recording = AudioRecorderMachine(hasClip = true).also { it.send(Event.START) }
        assertEquals(RemoteAction.STOP_RECORDING, recording.remoteAction(RemoteCommand.TOGGLE_PLAY_PAUSE, isPlaying = false))
        assertEquals(RemoteAction.STOP_RECORDING, recording.remoteAction(RemoteCommand.PAUSE, isPlaying = false))
        assertNull(recording.remoteAction(RemoteCommand.PLAY, isPlaying = false))
        val recorded = AudioRecorderMachine(hasClip = true)
        assertEquals(RemoteAction.STOP_PLAYBACK, recorded.remoteAction(RemoteCommand.TOGGLE_PLAY_PAUSE, isPlaying = true))
        assertNull(recorded.remoteAction(RemoteCommand.PLAY, isPlaying = true))
    }

    @Test
    fun dictationAppends() {
        assertEquals("こんにちは", DictationText.appending(" こんにちは ", ""))
        assertEquals("こんにちは", DictationText.appending("こんにちは", "  \n"))
        assertEquals("前\n後", DictationText.appending("後", "前"))
        assertEquals("前\n後", DictationText.appending("後", "前\n"))
        assertEquals("前", DictationText.appending("   ", "前"))
    }

    @Test
    fun speechVoiceFollowsUiLanguage() {
        assertEquals("ja-JP", SpeechVoice.languageTag("ja"))
        assertEquals("en-US", SpeechVoice.languageTag("en"))
        assertEquals("ja-JP", SpeechVoice.languageTag("fr"))
    }

    @Test
    fun claimsStopThePreviousHolder() {
        val claims = AudioClaims()
        val stopped = mutableListOf<String>()
        assertTrue(claims.claim("play", AudioClaims.Use.PLAYBACK) { stopped += "play" })
        assertTrue(claims.claim("rec", AudioClaims.Use.RECORDING) { stopped += "rec" })
        assertEquals(listOf("play"), stopped)
        // 録音中は読み上げない(前の持ち主も止めない)
        assertFalse(claims.claim("tts", AudioClaims.Use.SPEECH) { stopped += "tts" })
        assertEquals(AudioClaims.Use.RECORDING, claims.current)
        // 違う持ち主の release は効かない
        claims.release("play")
        assertEquals("rec", claims.owner)
        claims.release("rec")
        assertNull(claims.current)
        assertTrue(claims.claim("tts", AudioClaims.Use.SPEECH) { stopped += "tts" })
        assertTrue(claims.claim("dict", AudioClaims.Use.DICTATION) { stopped += "dict" })
        assertEquals(listOf("play", "tts"), stopped)
        claims.interrupt()
        assertEquals(listOf("play", "tts", "dict"), stopped)
        assertNull(claims.current)
    }

    @Test
    fun storeKeepsFilesInsideItsDirectory() {
        val dir = Files.createTempDirectory("audio").toFile()
        try {
            val store = LocalAudioStore(dir.resolve("metamon-audio"))
            assertNull(store.file("../x"))
            val tmp = store.temporaryRecording(dir, "t1").apply { writeText("aac") }
            val id = store.importFile(tmp, "clip1")
            assertEquals("clip1", id)
            assertTrue(store.exists("clip1"))
            assertFalse(tmp.exists())
            assertEquals("m4a", store.file("clip1")!!.extension)
            store.remove(listOf("clip1"))
            assertFalse(store.exists("clip1"))
        } finally {
            dir.deleteRecursively()
        }
    }
}
