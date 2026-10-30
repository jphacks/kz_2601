import Foundation
import MetamonDSL
import MetamonServices

/// aiButton generate のオンライン版(#417)。Renderer は端末内 AI が使えない時だけここを呼ぶ。
/// 判定は `AITextGate`(同意 → Pro)。モック(METAMON_MOCK)は `MockAIClient` が決まった文を返す。
/// 審査用ビルド(APP_STORE_REVIEW・#452 G4)はオンライン版(POST /v1/text)を持たない(同意文の「記録の中身は送らない」に合わせる)。
/// 端末内 AI(Foundation Models)はそのまま使え、無い端末では理由つきで無効になる。
#if APP_STORE_REVIEW
extension AppRuntime {
    var supportsOnlineAIText: Bool { false }

    func generateAIText(_ request: AITextRequest) async throws -> AITextResult {
        throw AITextError.unsupported
    }
}
#else
extension AppRuntime {
    var supportsOnlineAIText: Bool { true }

    func generateAIText(_ request: AITextRequest) async throws -> AITextResult {
        switch AITextGate.decide(consentGranted: hasAIConsent, isPro: proGate.isPro, enforced: proGate.isEnforced) {
        case .needsConsent:
            _ = requireAIConsent()
            throw AITextError.needsConsent
        case .needsPro:
            requestProForAIText()
            throw AITextError.needsPro
        case .send:
            return try await services.ai.text(request)
        }
    }

    /// 課金画面の差し込み口(1 か所)。#429 の `ProGate.requestPro` で出す(閉じるのは待たない。押し直しで再実行)。
    func requestProForAIText() {
        print("[Metamon] IAP: aiButton generate needs Pro (no on-device AI) → paywall")
        let feature = PaidFeature(id: "aiText", title: L10n.tr("オンラインの AI で書く"),
                                  reason: L10n.tr("この端末では AI を端末内で動かせないため、サーバーの AI を使います。"))
        Task { _ = await proGate.requestPro(.paidFeature([feature]), closeNote: L10n.tr("閉じると、この端末では AI で書くボタンは使えません。")) }
    }
}
#endif
