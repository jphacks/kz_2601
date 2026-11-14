package jp.kz2601.metamon.voice.stage

import androidx.annotation.StringRes
import jp.kz2601.metamon.R

/**
 * 生成中の経過段階(#894・iOS の `GeneratingStage`。docs/backend/ai-eval の予算に対応)。
 * - 0〜10 秒 NORMAL: 数秒ごとに文言を回す(create p50 6.6s / p95 8.7s)
 * - 10〜20 秒 SLOW: 「もう少しかかっています」
 * - 20〜30 秒 WARNING: 警告色
 * - 30 秒〜 FALLBACK: backend がテンプレートに切り替え中。キャンセルを枠付きで目立たせる
 * キャンセルは最初から小さく出す(言い間違いに気づいたらすぐ止められる)。
 */
enum class GeneratingStage {
    NORMAL, SLOW, WARNING, FALLBACK;

    /** 警告色にするか */
    val warns: Boolean get() = this == WARNING || this == FALLBACK

    /** キャンセルを枠付きで目立たせるか。それまでは小さい文字のボタン */
    val cancelIsProminent: Boolean get() = this == FALLBACK

    /** 段階の文言。create の最初の段は 3 秒ごとに回す */
    @StringRes
    fun message(elapsedSeconds: Double, edit: Boolean): Int = when (this) {
        NORMAL -> if (edit) R.string.voice_gen_editing else NORMAL_MESSAGES[(elapsedSeconds / 3).toInt().coerceAtLeast(0) % NORMAL_MESSAGES.size]
        SLOW -> R.string.voice_gen_slow
        WARNING -> R.string.voice_gen_warning
        FALLBACK -> R.string.voice_gen_fallback
    }

    /** 待ち時間の目安。create の最初の段だけ(過ぎたら段階の文言に任せる) */
    @StringRes
    fun estimate(edit: Boolean): Int? = if (this == NORMAL && !edit) R.string.voice_gen_usually else null

    companion object {
        fun of(elapsedSeconds: Double): GeneratingStage = when {
            elapsedSeconds < 10 -> NORMAL
            elapsedSeconds < 20 -> SLOW
            elapsedSeconds < 30 -> WARNING
            else -> FALLBACK
        }

        val NORMAL_MESSAGES = listOf(
            R.string.voice_gen_thinking,
            R.string.voice_gen_deciding,
            R.string.voice_gen_building,
            R.string.voice_gen_polishing,
        )

        /** 見出しに出さない切替キーワード(iOS の `VoiceIntent.newAppPrefixes` と同じ語) */
        private val NEW_APP_PREFIXES = listOf("別のアプリ", "別アプリ", "新しいアプリ", "新規アプリ", "違うアプリ", "次のアプリ", "べつのアプリ", "あたらしいアプリ")

        /** 見出しの依頼文。「別のアプリ。」などの切替キーワードは出さない */
        fun displayTranscript(transcript: String): String {
            val p = NEW_APP_PREFIXES.firstOrNull { transcript.startsWith(it) } ?: return transcript
            val rest = transcript.drop(p.length).trim { it.isWhitespace() || !it.isLetterOrDigit() }
            return rest.ifEmpty { transcript }
        }
    }
}
