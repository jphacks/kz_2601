import Foundation
import MetamonServices

/// 「今日の残り N」(#630 M1・X3)の出し分け。単位はサーバーの重み(新規 3・修正 2・チャット 1)なので「回」と言わない。
enum RemainingTodayDisplay: Equatable {
    /// まだ分からない・日付が変わって古い
    case hidden
    case normal(Int, pro: Bool)
    /// 少ない(`lowThreshold` 以下)。目立たせる
    case low(Int, pro: Bool)
    /// 0。既存の上限のカード(#280)に渡す
    case exhausted(RateLimitInfo)

    /// 新しく作るのがあと 2 回分を切ったら少ない扱い
    static let lowThreshold = RemainingToday.Weight.create * 2

    static func of(_ value: RemainingToday?, now: Date, isPro: Bool, isAnonymous: Bool) -> RemainingTodayDisplay {
        guard let value, !value.isStale(now: now) else { return .hidden }
        if value.remaining <= 0 {
            let tier = isPro ? "pro" : (isAnonymous ? "anonymous" : "signed_in")
            return .exhausted(RateLimitInfo(scope: .user, tier: tier, limit: value.limit, remaining: 0, resetAt: value.resetAt))
        }
        return value.remaining <= lowThreshold ? .low(value.remaining, pro: isPro) : .normal(value.remaining, pro: isPro)
    }

    static func label(_ n: Int) -> String { L10n.tr("今日の残り \(n)") }

    /// VoiceOver で読む 1 文
    var accessibilityLabel: String {
        switch self {
        case .hidden: ""
        case .normal(let n, let pro): (pro ? L10n.tr("Pro。") : "") + Self.label(n)
        case .low(let n, let pro): (pro ? L10n.tr("Pro。") : "") + Self.label(n) + L10n.tr("。少なくなっています")
        case .exhausted: Self.label(0)
        }
    }

    /// 何で減るか・いつ戻るか
    static func hint(resetAt: Date?) -> String {
        let when = resetAt.map { ErrorCopy.resetTime($0) } ?? L10n.tr("明日")
        let c = RemainingToday.Weight.create, e = RemainingToday.Weight.edit, t = RemainingToday.Weight.chat
        return L10n.pick(L10n.tr("新しく作ると \(c)、直すと \(e)、相談すると \(t) 減ります。\(when)に戻ります"),
                         review: L10n.tr("新しく用意すると \(c)、直すと \(e)、相談すると \(t) 減ります。\(when)に戻ります"))
    }
}
