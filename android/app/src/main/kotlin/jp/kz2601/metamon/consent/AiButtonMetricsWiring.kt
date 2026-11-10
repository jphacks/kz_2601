package jp.kz2601.metamon.consent

import jp.kz2601.metamon.renderer.components.ai.AiButtonMetric
import jp.kz2601.metamon.services.consent.AiButtonMetricsRecorder

/** 描画器の集計の 1 件を貯める(iOS の `AIButtonMetrics+Wiring`・#888)。送るのは 1 日 1 回 [AiButtonMetricsRecorder.flush]。 */
fun AiButtonMetricsRecorder.record(event: AiButtonMetric) = when (event) {
    is AiButtonMetric.Shown -> recordShown(event.availability)
    is AiButtonMetric.Ran -> recordRan(event.task, event.failure)
}
