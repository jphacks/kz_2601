package jp.kz2601.metamon.desktop.voice

import jp.kz2601.metamon.desktop.ActionContext
import jp.kz2601.metamon.desktop.BookLookupClient
import jp.kz2601.metamon.desktop.DesktopActionHost
import jp.kz2601.metamon.desktop.DesktopSession
import jp.kz2601.metamon.desktop.Examples
import jp.kz2601.metamon.desktop.MockStore
import jp.kz2601.metamon.desktop.Notifier
import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.dsl.Action
import jp.kz2601.metamon.dsl.ActionType
import jp.kz2601.metamon.dsl.Notification
import jp.kz2601.metamon.whisper.ModelException
import jp.kz2601.metamon.whisper.ModelFailure
import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File
import java.nio.file.Files

/** openVoice の録音 → 文字起こしの流れ(マイクと whisper は偽物)。 */
class VoiceInputTest {
    /** 録った音として [samples] を返すマイク。[fail] があれば開けない。 */
    private class FakeMic(var samples: FloatArray = speech(), var fail: MicException.Reason? = null) : Microphone {
        var started = 0
        var lastDevice: String? = null
        override fun devices() = listOf("既定", "USB マイク")
        override fun start(device: String?): Microphone.Recording {
            fail?.let { throw MicException(it) }
            started++
            lastDevice = device
            return object : Microphone.Recording {
                override fun stop() = samples
                override fun cancel() {}
            }
        }
    }

    /** Transcriber の偽物。[gate] を渡すと、それが終わるまで文字にし終えない。 */
    private class FakeTranscriber(
        var text: String = "牛乳を買う",
        var error: Exception? = null,
        override val isAvailable: Boolean = true,
        override var pendingDownloadMegabytes: Int? = null,
        val gate: CompletableDeferred<Unit>? = null,
    ) : Transcriber {
        val calls = mutableListOf<Pair<Int, String>>()
        override suspend fun transcribe(samples: FloatArray, language: String): String {
            calls += samples.size to language
            gate?.await()
            error?.let { throw it }
            return text
        }
    }

    private fun VoiceInput.pressAndWait(): Pair<String, String?> {
        var posted: String? = null
        val now = toggle { posted = it }
        runBlocking { job?.join() }
        return now to posted
    }

    @Test
    fun flagIsOffByDefault() {
        assertFalse(VoiceFlag.enabled({ null }, { null }))
        assertTrue(VoiceFlag.enabled({ null }, mapOf("TAILORIT_VOICE" to "1")::get))
        assertTrue(VoiceFlag.enabled(mapOf("tailorit.voice" to "true")::get, { null }))
        assertFalse(VoiceFlag.enabled({ null }, mapOf("TAILORIT_VOICE" to "0")::get))
    }

    @Test
    fun recordThenTranscribeOnDevice() {
        val mic = FakeMic()
        val tr = FakeTranscriber()
        val v = VoiceInput(mic, tr, language = { "ja" }, device = "USB マイク")
        assertEquals(Strings["desktop_voice_recording"], v.toggle { })
        assertEquals(VoiceInput.State.RECORDING, v.state)
        assertEquals("USB マイク", mic.lastDevice)
        val (now, posted) = v.pressAndWait()
        assertEquals(Strings["desktop_voice_transcribing"], now)
        assertEquals(Strings.format("desktop_voice_heard", "牛乳を買う"), posted)
        assertEquals(listOf(16_000 to "ja"), tr.calls)
        assertEquals(VoiceInput.State.IDLE, v.state)
    }

    @Test
    fun firstUseSaysTheModelSize() {
        val v = VoiceInput(FakeMic(), FakeTranscriber(pendingDownloadMegabytes = 60))
        v.toggle { }
        assertEquals(Strings.format("desktop_voice_downloading", 60), v.pressAndWait().first)
    }

    @Test
    fun pressWhileTranscribingIsIgnored() {
        val gate = CompletableDeferred<Unit>()
        val tr = FakeTranscriber(gate = gate)
        val v = VoiceInput(FakeMic(), tr)
        v.toggle { }
        v.toggle { }
        assertEquals(VoiceInput.State.TRANSCRIBING, v.state)
        assertEquals(Strings["desktop_voice_busy"], v.toggle { })
        gate.complete(Unit)
        runBlocking { v.job?.join() }
        assertEquals(VoiceInput.State.IDLE, v.state)
        assertEquals(1, tr.calls.size)
    }

    @Test
    fun noLibraryOrNoMicSaysWhyWithoutRecording() {
        val mic = FakeMic()
        assertEquals(Strings["desktop_voice_unavailable"], VoiceInput(mic, FakeTranscriber(isAvailable = false)).toggle { })
        assertEquals(0, mic.started)
        val noMic = VoiceInput(FakeMic(fail = MicException.Reason.NO_DEVICE), FakeTranscriber())
        assertEquals(Strings["desktop_voice_no_mic"], noMic.toggle { })
        assertEquals(VoiceInput.State.IDLE, noMic.state)
        assertEquals(Strings["desktop_voice_mic_busy"], VoiceInput(FakeMic(fail = MicException.Reason.BUSY), FakeTranscriber()).toggle { })
    }

    @Test
    fun blockedMicTooShortAndSilenceAreNotTranscribed() {
        val tr = FakeTranscriber()
        // OS がマイクを止めていると全部 0 が届く(Windows のプライバシーの設定)
        val zeros = VoiceInput(FakeMic(FloatArray(16_000)), tr)
        zeros.toggle { }
        assertEquals(Strings["desktop_voice_mic_denied"], zeros.toggle { })
        val short = VoiceInput(FakeMic(speech(seconds = 0.2)), tr)
        short.toggle { }
        assertEquals(Strings["desktop_voice_too_short"], short.toggle { })
        val quiet = VoiceInput(FakeMic(FloatArray(16_000) { 0.0001f }), tr)
        quiet.toggle { }
        assertEquals(Strings["voice_b_silent"], quiet.toggle { })
        assertTrue(tr.calls.isEmpty())
    }

    @Test
    fun failuresBecomeMessages() {
        fun post(tr: Transcriber): String? = VoiceInput(FakeMic(), tr).run { toggle { }; pressAndWait().second }
        assertEquals(Strings["voice_b_not_heard"], post(FakeTranscriber(text = "  ")))
        assertEquals(Strings["desktop_voice_model_network"], post(FakeTranscriber(error = ModelException(ModelFailure.Network))))
        assertEquals(Strings.format("desktop_voice_model_space", 110), post(FakeTranscriber(error = ModelException(ModelFailure.NotEnoughStorage(109_707_625, 1)))))
        assertEquals(Strings["desktop_voice_failed"], post(FakeTranscriber(error = IllegalStateException("x"))))
    }

    @Test
    fun openVoiceUsesVoiceInputOnlyWhenGiven() {
        val silent = object : Notifier {
            override fun schedule(n: Notification) = ""
            override fun cancel(id: String): String? = null
        }
        val dir: File = Files.createTempDirectory("voice").toFile()
        val voice = VoiceInput(FakeMic(), FakeTranscriber(text = "卵"))
        val s = DesktopSession(Examples.load("shopping"), MockStore())
        s.host = DesktopActionHost(BookLookupClient.mock, dir, silent, voice)
        fun press() = s.dispatch(Action(type = ActionType.OPEN_VOICE), ActionContext(s.top, s.form(s.top)))
        press()
        assertEquals(Strings["desktop_voice_recording"], s.message)
        press()
        runBlocking { voice.job?.join() }
        assertEquals(Strings.format("desktop_voice_heard", "卵"), s.message)

        // 既定(フラグが無い)は今までどおり使えない旨
        val off = DesktopSession(Examples.load("shopping"), MockStore())
        off.host = DesktopActionHost(BookLookupClient.mock, dir, silent, voice = null)
        off.dispatch(Action(type = ActionType.OPEN_VOICE), ActionContext(off.top, off.form(off.top)))
        assertEquals(DesktopActionHost.UNAVAILABLE_VOICE, off.message)
    }

    @Test
    fun modelsLiveUnderAppDataAndCanBeOverridden() {
        assertEquals(File("/data/tailorit", "models"), VoiceModels.dir(File("/data/tailorit")))
        assertNull(VoiceModels.override({ null }, { null }))
        assertEquals(File("/m/ggml.bin"), VoiceModels.override({ null }, mapOf("TAILORIT_WHISPER_MODEL" to "/m/ggml.bin")::get))
        // Android と同じモデル(版とハッシュを固定)
        assertEquals("ggml-base-q5_1.bin", VoiceModels.DEFAULT.fileName)
        assertTrue(VoiceModels.DEFAULT.url.contains("/resolve/5359861c739e955e79d9a303bcbc70fb988958b1/"))
    }

    companion object {
        /** 声の代わり(440Hz・振幅 0.3)。 */
        fun speech(seconds: Double = 1.0) = FloatArray((16_000 * seconds).toInt()) { (0.3 * kotlin.math.sin(2 * Math.PI * 440 * it / 16_000.0)).toFloat() }
    }
}
