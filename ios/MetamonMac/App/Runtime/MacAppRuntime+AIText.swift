import Foundation
import MetamonDSL
import MetamonRenderer
import MetamonServices

/// aiButton・aiGenerate・aiChat(#962)。部品は Renderer の iOS と同じもの: 端末内 AI(Foundation Models・macOS 26+)を先に使い、
/// 使えない Mac・長い文・写真・aiChat はここのオンライン版に回す(iOS の `AppRuntime+AIText` と同じ道)。
/// 送る前に同意(#246)を確かめ、無ければ同意の説明を出して送らない(押し直しで送る・iOS と同じ)。
/// AI のクライアントは会話と同じ `MacComposeSession.gatedAI()`(Firebase が無い間はモック・#930)。
/// 課金(Pro)の画面は Mac にまだ無いので出さない。サーバーが Pro を求めたら部品の下に理由を出す。
/// 審査用ビルド(APP_STORE_REVIEW)は iOS と同じくオンライン版を持たない(aiChat は部品ごと外れる)。
extension MacAppRuntime: AITextHost {
    #if APP_STORE_REVIEW
    var supportsOnlineAIText: Bool { false }

    func generateAIText(_ request: AITextRequest) async throws -> AITextResult {
        throw AITextError.unsupported
    }

    func recordAIButton(_ event: AIButtonMetric.Event) {}
    #else
    var supportsOnlineAIText: Bool { true }

    func generateAIText(_ request: AITextRequest) async throws -> AITextResult {
        guard isAIConsentGranted() else {
            needsAIConsent = true
            throw AITextError.needsConsent
        }
        return try await ai.text(request.inApp(appId))
    }

    /// 端末内 AI の集計(#446)。設定の「AI ボタンの利用状況を送る」で止められる。送るのは appconfig につながってから(#930)
    func recordAIButton(_ event: AIButtonMetric.Event) {
        AIButtonMetricsRecorder.shared.record(event)
    }
    #endif
}
