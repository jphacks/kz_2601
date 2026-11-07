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
    /// - sharedApp: 共有の記録を持つアプリの中から。参加者なら作成者の Pro で通るので、Pro でなくてもサーバーに判定させる
    ///   (作成者が Pro でなければ 402 `creator_needs_pro`、参加者でなければ 402 → 課金画面)
    public static func decide(consentGranted: Bool, isPro: Bool, enforced: Bool, sharedApp: Bool = false) -> Decision {
        guard consentGranted else { return .needsConsent }
        guard isPro || !enforced || sharedApp else { return .needsPro }
        return .send
    }
}
