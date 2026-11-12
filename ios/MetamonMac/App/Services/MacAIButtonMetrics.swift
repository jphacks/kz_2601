// 審査用ビルドは集計を送らない(iOS の `LiveWiring.aiButtonMetrics` と同じ・#446)
#if !APP_STORE_REVIEW
import AppKit
import MetamonServices

extension MacServices {
    /// 端末内 AI の集計(#446)の送り先。appconfig の URL がある時だけ(モックでは貯めるだけ)
    var aiButtonMetrics: (any AIButtonMetricsSender)? {
        appConfigURL.map { HTTPAIButtonMetricsSender(baseURL: $0, token: token) }
    }
}

/// aiButton の端末内 AI の集計を送る時点(#1327)。iOS の `flushAIButtonMetrics` と同じく、起動と前面に戻った時に
/// 前の日までの分を送る(送るかどうかは設定の「AI ボタンの利用状況を送る」・`AIButtonMetricsRecorder` が見る)。
@MainActor
enum MacAIButtonMetrics {
    private static var observer: NSObjectProtocol?

    /// 起動時に 1 回。アプリが前面になるたび(起動直後も)送る
    static func install(services: MacServices = .shared) {
        guard observer == nil else { return }
        observer = NotificationCenter.default.addObserver(forName: NSApplication.didBecomeActiveNotification, object: nil, queue: .main) { _ in
            Task { @MainActor in await flush(sender: services.aiButtonMetrics) }
        }
    }

    static func flush(sender: (any AIButtonMetricsSender)?, recorder: AIButtonMetricsRecorder = .shared) async {
        guard let sender else { return }
        await recorder.flush(sender: sender)
    }
}
#endif
