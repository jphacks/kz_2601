import Foundation
import MetamonDSL
import MetamonServices

/// aiButton の端末内 AI の集計(#446)。Renderer が渡したラベルを `AIButtonMetricsRecorder` に貯め、前の日までの分を 1 日 1 回送る。
/// 審査用ビルド(APP_STORE_REVIEW)は数えない・送らない(栄養ラベルに「使用状況データ」が増えるため)。
#if APP_STORE_REVIEW
extension AppRuntime {
    func recordAIButton(_ event: AIButtonMetric.Event) {}
    func flushAIButtonMetrics() async {}
}
#else
extension AppRuntime {
    func recordAIButton(_ event: AIButtonMetric.Event) {
        AIButtonMetricsRecorder.shared.record(event)
    }

    /// 起動・前面に戻った時に呼ぶ。appconfig が live でなければ何もしない(モックでは貯めるだけ)。
    func flushAIButtonMetrics() async {
        guard let sender = services.aiButtonMetrics else { return }
        await AIButtonMetricsRecorder.shared.flush(sender: sender)
    }
}
#endif
