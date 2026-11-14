package jp.kz2601.metamon.renderer.components.nearby

import androidx.compose.runtime.staticCompositionLocalOf
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.renderer.DSLRuntime
import jp.kz2601.metamon.renderer.components.media.PermissionPrompt

/** 近距離共有(#36・iOS の `NearbyHost`)。持たないホスト(プレビュー・テスト)ではボタンを押しても何もしない。 */
interface NearbyHost {
    /** `nearbyShare`: 近くの端末を探すシートを開き、選んだ相手に [records](entity の保存フィールドのみ)を送る。 */
    fun shareNearby(entity: String, records: List<Map<String, JSONValue>>)

    /** `nearbyShare` mode:passby(すれ違い通信・#1383)の通信側。null なら何も通信しない表示だけ。 */
    val passby: NearbyPassbyController? get() = null
}

val DSLRuntime.nearby: NearbyHost? get() = this as? NearbyHost

/**
 * 近くのデバイスの権限を聞く窓口。:app が権限の窓口(理由 → OS のダイアログ → 設定へ)で差し替える。
 * 既定(プレビュー・テスト)は聞かずに進む(ホストが無いので何も送らない)。
 */
val LocalNearbyPermission = staticCompositionLocalOf<PermissionPrompt> { { onResult -> { onResult(true) } } }
