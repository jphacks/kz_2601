package jp.kz2601.metamon.feedback

/**
 * トーストと「削除しました・元に戻す」をどこに出すか(#1106・iOS の `ToastHost(onModal:)`)。
 * 音声のシート(ModalBottomSheet)は別のウィンドウなので、開いている間は画面側に出すとシートの裏に隠れる。
 * その間はシートの中だけに出し、二重には出さない。
 */
enum class ToastPlacement {
    SCREEN,
    VOICE_SHEET,
    ;

    companion object {
        /** 音声のシートを出しているか(話しかける前の画面は全画面の音声画面なのでシートを使わない) */
        fun voiceSheetShown(isInitial: Boolean, voiceOpen: Boolean): Boolean = !isInitial && voiceOpen

        fun of(isInitial: Boolean, voiceOpen: Boolean): ToastPlacement =
            if (voiceSheetShown(isInitial, voiceOpen)) VOICE_SHEET else SCREEN
    }
}
