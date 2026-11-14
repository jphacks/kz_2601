package jp.kz2601.metamon.renderer.components.scan

import jp.kz2601.metamon.dsl.records.JSONValue

/** 読み取り 1 回の結果。 */
sealed interface QrScanResult {
    /** 読めた。[raw] はコードの中身そのまま(null は中身を文字にできないコード)。 */
    data class Found(val raw: String?) : QrScanResult

    /** 閉じた(戻る・×)。 */
    data object Canceled : QrScanResult

    /** 読み取りの画面を出せなかった(Play 開発者サービスのモジュールが無い・落とせない等)。 */
    data object Failed : QrScanResult
}

/** 結果を受けて何をするか。View から切り離して JVM でテストする。 */
sealed interface QrScanOutcome {
    /** field にこの値を書く(iOS と同じ `.string(code)`)。 */
    data class Write(val value: JSONValue) : QrScanOutcome

    /** 何もしない。 */
    data object Keep : QrScanOutcome

    /** 手入力の欄を出す(この端末ではカメラで読めない)。 */
    data object ManualOnly : QrScanOutcome
}

/** `qrScanner` の値の扱い(iOS の `QrScannerLeaf`)。 */
object QrScanRules {
    /**
     * 読めた中身は加工せずに文字列で入れる(iOS は前後の空白も落とさない)。空・文字にできないコードは書かない。
     * 1 回開くと 1 件だけ返す(iOS の `found` と同じく、同じ値を続けて書かない)。
     */
    fun outcome(result: QrScanResult): QrScanOutcome = when (result) {
        is QrScanResult.Found -> result.raw?.takeIf { it.isNotEmpty() }?.let { QrScanOutcome.Write(JSONValue.Str(it)) } ?: QrScanOutcome.Keep
        QrScanResult.Canceled -> QrScanOutcome.Keep
        QrScanResult.Failed -> QrScanOutcome.ManualOnly
    }

    /** 読んだ値の表示(空なら出さない)。 */
    fun preview(value: JSONValue?): String? = (value ?: JSONValue.Null).displayString.takeIf { it.isNotEmpty() }
}
