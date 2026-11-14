import Foundation
import MetamonDSL

/// サーバーが Tailor It Pro を求めた 402 `pro_required`(#316)。`details.features` = 要る有料の機能、
/// `details.reason == "creator_needs_pro"` = 呼び出した人ではなくアプリの作成者の Pro が要る(共有の記録の書き込み)。
/// 審査用の版は有料の機能を持たないので当たらない想定だが、判定は外さない(当たった時に課金画面ではなく案内を出すため)。
public struct ProRequired: Sendable, Equatable {
    public static let code = "pro_required"
    public static let creatorNeedsProReason = "creator_needs_pro"

    public var features: [PaidFeature]
    /// アプリの作成者の Pro が要る(参加者は払っても直らないので課金画面を出さない)
    public var creatorNeedsPro: Bool

    public init(features: [PaidFeature], creatorNeedsPro: Bool = false) {
        self.features = features
        self.creatorNeedsPro = creatorNeedsPro
    }

    public init?(_ error: Error) {
        guard let api = error as? APIError, api.status == 402, api.code == Self.code,
              case .server(_, _, _, let details) = api else { return nil }
        var features: [PaidFeature] = []
        if case .array(let items)? = details["features"] {
            for item in items {
                guard case .object(let o) = item, case .string(let id)? = o["id"] else { continue }
                var title = id, reason = ""
                if case .string(let t)? = o["title"] { title = t }
                if case .string(let r)? = o["reason"] { reason = r }
                features.append(PaidFeature(id: id, title: title, reason: reason))
            }
        }
        self.init(features: features, creatorNeedsPro: api.reason == Self.creatorNeedsProReason)
    }
}
