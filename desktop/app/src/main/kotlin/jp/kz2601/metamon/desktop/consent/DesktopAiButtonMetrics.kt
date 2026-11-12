package jp.kz2601.metamon.desktop.consent

import androidx.compose.runtime.staticCompositionLocalOf
import jp.kz2601.metamon.services.consent.AiButtonMetricsRecorder
import jp.kz2601.metamon.services.consent.AiButtonMetricsSender
import jp.kz2601.metamon.services.consent.AiConsentStore

/**
 * aiButton の集計(#2817・docs/analytics/ai-button-metrics.md)。数え方・送り方は Android の [AiButtonMetricsRecorder] そのまま
 * (端末のローカル日ごとに貯め、前の日までの分を 1 日 1 回まとめて送る)。AI の同意が無い間は数えず送らない。
 * aiButton の部品(#2387)は描いた時に [shown]、動かした時に [ran] を呼ぶ。
 *
 * @param sender null ならモック(送らない)
 */
class DesktopAiButtonMetrics(
    private val recorder: AiButtonMetricsRecorder,
    private val consent: AiConsentStore,
    private val sender: AiButtonMetricsSender?,
) {
    /** ボタンを描いた時の可用性(Android と同じラベル。知らない値は other) */
    fun shown(availability: String) {
        if (consent.isGranted) recorder.recordShown(availability)
    }

    /** 1 回動かした。[failure] = null は答えを出せた */
    fun ran(task: String?, failure: String?) {
        if (consent.isGranted) recorder.recordRan(task, failure)
    }

    /** 前の日までの分を送る(起動時)。同意が無い・送り先が無い時は送らない */
    suspend fun flush() {
        val s = sender ?: return
        if (consent.isGranted) recorder.flush(s)
    }
}

/** aiButton の部品から引く口。null は数えない(テスト・適合の描画) */
val LocalAiButtonMetrics = staticCompositionLocalOf<DesktopAiButtonMetrics?> { null }
