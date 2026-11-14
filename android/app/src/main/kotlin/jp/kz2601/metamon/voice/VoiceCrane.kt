package jp.kz2601.metamon.voice

import jp.kz2601.metamon.ui.CranePose

/**
 * 音声画面の見出しに置くツル(#1132・iOS の `VoiceInputView.heading`)。
 * 話しかける前の画面だけ・吹き出しの画面では出さない・縦に余裕が無い時(横向き)は出さない(iOS #625)。
 * ポーズは iOS と同じく状態によらず happy(聞いている・考え中はマイクと状態の文言が示す)。
 */
object VoiceCrane {
    /** 横向きの電話などで縦が狭いとみなす高さ(iOS の verticalSizeClass == .compact に相当) */
    const val COMPACT_HEIGHT_DP = 480

    fun isCompactHeight(screenHeightDp: Int): Boolean = screenHeightDp < COMPACT_HEIGHT_DP

    /** 出すポーズ。出さないなら null */
    fun pose(state: VoiceUiState, compactHeight: Boolean): CranePose? =
        if (state.isInitial && !state.showsChat && !compactHeight) CranePose.HAPPY else null
}
