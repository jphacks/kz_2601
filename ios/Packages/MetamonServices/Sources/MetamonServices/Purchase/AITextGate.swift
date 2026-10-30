import Foundation

/// aiButton generate のオンライン版(#417)を送ってよいか。純粋関数のみ。
/// 端末内 AI は無料(Renderer が先に使う)。ここに来るのは端末内 AI が使えない時だけで、サーバーの費用がかかるので Pro の機能。
public enum AITextGate {
    public enum Decision: Equatable, Sendable {
        case send
        /// AI へのデータ送信の同意画面を出す
        case needsConsent
        /// 課金画面を出す
        case needsPro
    }

    /// - enforced: 課金の制限が有効か(Debug の既定は無効 = `ProGate.isEnforced`)
    public static func decide(consentGranted: Bool, isPro: Bool, enforced: Bool) -> Decision {
        guard consentGranted else { return .needsConsent }
        guard isPro || !enforced else { return .needsPro }
        return .send
    }
}
