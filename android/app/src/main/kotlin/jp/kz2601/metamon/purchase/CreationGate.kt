package jp.kz2601.metamon.purchase

import jp.kz2601.metamon.services.http.PaidFeature

/**
 * 新規作成の前後で課金を通す口(#1261・iOS の `ProGate` の `admitNewApp` / `recordNewApp` / `requestPro`)。
 * 音声の画面([jp.kz2601.metamon.voice.VoiceViewModel])が使い、[ProStore] が実装する。
 */
interface CreationGate {
    /** 1 日の無料作成の回数(カードの見出しに出す) */
    val dailyFreeLimit: Int

    /** 新規作成の前。枠切れなら課金画面を出し、閉じるまで待つ。戻り値 = 作ってよいか */
    suspend fun admitNewApp(): Boolean

    /** 新規作成ができた時(失敗・取り消しは数えない) */
    fun recordNewApp()

    /** 402 `pro_required` の時に課金画面を出し、閉じるまで待つ。戻り値 = 閉じた時に Pro か */
    suspend fun requestPro(features: List<PaidFeature>): Boolean
}
