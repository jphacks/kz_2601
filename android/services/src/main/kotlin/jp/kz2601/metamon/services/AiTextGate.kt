package jp.kz2601.metamon.services

/**
 * aiButton のオンライン版を送ってよいか(iOS の `AITextGate`)。純粋関数のみ。
 * 端末内 AI は無料(描画器が先に使う)。ここに来るのは端末内 AI が使えない時だけで、サーバーの費用がかかるので Pro の機能。
 */
object AiTextGate {
    enum class Decision {
        SEND,

        /** AI へのデータ送信の同意が要る */
        NEEDS_CONSENT,

        /** 課金画面を出す */
        NEEDS_PRO,
    }

    /**
     * - enforced: 課金の制限が有効か(Debug の既定は無効)
     * - sharedApp: 共有の記録を持つアプリの中から。参加者は作成者の Pro で通るので、Pro でなくてもサーバーに判定させる
     *   (作成者が Pro でなければ 402 `creator_needs_pro`、参加者でなければ 402 → 課金画面)
     */
    fun decide(consentGranted: Boolean, isPro: Boolean, enforced: Boolean, sharedApp: Boolean = false): Decision = when {
        !consentGranted -> Decision.NEEDS_CONSENT
        !(isPro || !enforced || sharedApp) -> Decision.NEEDS_PRO
        else -> Decision.SEND
    }
}
