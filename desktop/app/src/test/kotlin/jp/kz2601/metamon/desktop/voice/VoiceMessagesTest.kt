package jp.kz2601.metamon.desktop.voice

import jp.kz2601.metamon.desktop.i18n.Lang
import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.whisper.ModelFailure
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** 音声入力の案内の文: 録った音の出し分け(silenceMessage)とモデルの失敗ごとの文(modelFailureMessage)。 */
class VoiceMessagesTest {
    /** 0.5 秒(16kHz)。これ未満は短すぎ。 */
    private val minimum = 8_000

    @Test
    fun silenceMessage_emptyAndShortAreTooShort() = Strings.withLang(Lang.JA) {
        assertEquals(Strings["desktop_voice_too_short"], VoiceInput.silenceMessage(FloatArray(0)))
        // 大きな声でも短すぎれば文字にしない
        assertEquals(Strings["desktop_voice_too_short"], VoiceInput.silenceMessage(FloatArray(minimum - 1) { 0.5f }))
    }

    @Test
    fun silenceMessage_allZeroIsMicDenied() = Strings.withLang(Lang.JA) {
        assertEquals(Strings["desktop_voice_mic_denied"], VoiceInput.silenceMessage(FloatArray(minimum)))
        assertEquals(Strings["desktop_voice_mic_denied"], VoiceInput.silenceMessage(FloatArray(16_000 * 3)))
    }

    @Test
    fun silenceMessage_quietIsSilent() = Strings.withLang(Lang.JA) {
        assertEquals(Strings["voice_b_silent"], VoiceInput.silenceMessage(FloatArray(minimum) { 0.0005f }))
        // 負の小さな振れも無音
        assertEquals(Strings["voice_b_silent"], VoiceInput.silenceMessage(FloatArray(minimum) { if (it % 2 == 0) -0.0009f else 0f }))
    }

    @Test
    fun silenceMessage_normalVoiceIsTranscribed() {
        assertNull(VoiceInput.silenceMessage(FloatArray(minimum) { 0.001f }))
        assertNull(VoiceInput.silenceMessage(FloatArray(16_000) { if (it == 100) -0.4f else 0f }))
        assertNull(VoiceInput.silenceMessage(FloatArray(16_000) { kotlin.math.sin(it / 10.0).toFloat() * 0.3f }))
    }

    private val failures = listOf(ModelFailure.Network, ModelFailure.Checksum, ModelFailure.Storage, ModelFailure.NotEnoughStorage(150_000_000, 1))

    @Test
    fun modelFailureMessage_eachKindHasItsOwnTextInBothLanguages() {
        for (lang in listOf(Lang.JA, Lang.EN)) {
            Strings.withLang(lang) {
                val texts = failures.map { VoiceInput.modelFailureMessage(it) }
                // 鍵の名前のままでない(表に文がある)
                texts.forEach { assertTrue("$lang: $it", it.isNotBlank() && !it.startsWith("desktop_voice_")) }
                assertEquals("$lang: 種類ごとに違う文", texts.size, texts.toSet().size)
            }
        }
        failures.forEach { f ->
            assertNotEquals(
                Strings.withLang(Lang.JA) { VoiceInput.modelFailureMessage(f) },
                Strings.withLang(Lang.EN) { VoiceInput.modelFailureMessage(f) },
            )
        }
    }

    @Test
    fun modelFailureMessage_mapsToKeys() = Strings.withLang(Lang.EN) {
        assertEquals(Strings["desktop_voice_model_network"], VoiceInput.modelFailureMessage(ModelFailure.Network))
        assertEquals(Strings["desktop_voice_model_broken"], VoiceInput.modelFailureMessage(ModelFailure.Checksum))
        assertEquals(Strings["desktop_voice_model_storage"], VoiceInput.modelFailureMessage(ModelFailure.Storage))
    }

    @Test
    fun modelFailureMessage_spaceRoundsUpToMegabytes() {
        Strings.withLang(Lang.EN) {
            assertEquals("Not enough space for the voice model (1 MB needed)", VoiceInput.modelFailureMessage(ModelFailure.NotEnoughStorage(1, 0)))
            assertEquals("Not enough space for the voice model (1 MB needed)", VoiceInput.modelFailureMessage(ModelFailure.NotEnoughStorage(1_000_000, 0)))
            assertEquals("Not enough space for the voice model (2 MB needed)", VoiceInput.modelFailureMessage(ModelFailure.NotEnoughStorage(1_000_001, 0)))
        }
        Strings.withLang(Lang.JA) {
            assertEquals("音声モデルを置く空きが足りません(110 MB 要ります)", VoiceInput.modelFailureMessage(ModelFailure.NotEnoughStorage(109_707_625, 0)))
        }
    }
}
