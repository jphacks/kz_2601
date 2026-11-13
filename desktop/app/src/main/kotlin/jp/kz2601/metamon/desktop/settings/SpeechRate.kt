package jp.kz2601.metamon.desktop.settings

import jp.kz2601.metamon.services.consent.KeyValueStore

/**
 * 読み上げ(speakButton)の速さ(#3002)。iOS の「読み上げの声と速さ」の速さだけ。遅い / ふつう / 速いの 3 段。
 * `SpeechSynth` の実装(Windows の SAPI・Linux の spd-say)に値を渡す。
 */
enum class SpeechRate(val storageValue: String) {
    SLOW("slow"),
    NORMAL("normal"),
    FAST("fast"),
    ;

    /** SAPI の `SpeechSynthesizer.Rate`(-10〜10。0 が標準)。 */
    val sapiRate: Int
        get() = when (this) {
            SLOW -> -3
            NORMAL -> 0
            FAST -> 3
        }

    /** `spd-say -r` の値(-100〜100。0 が標準)。 */
    val spdSayRate: Int
        get() = when (this) {
            SLOW -> -40
            NORMAL -> 0
            FAST -> 40
        }

    companion object {
        val DEFAULT = NORMAL
        const val KEY = "speech.rate"

        /** 保存された文字列から。無い・知らない値は [DEFAULT]。 */
        fun parse(raw: String?): SpeechRate = entries.firstOrNull { it.storageValue == raw?.trim()?.lowercase() } ?: DEFAULT

        fun load(store: KeyValueStore): SpeechRate = parse(store.getString(KEY))

        fun save(store: KeyValueStore, rate: SpeechRate) = store.putString(KEY, rate.storageValue)
    }
}
