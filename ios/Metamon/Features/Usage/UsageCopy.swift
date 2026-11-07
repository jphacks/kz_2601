import Foundation
import MetamonServices

/// 使っている量・上限に近い時の文言(#317)。何の上限かは上限に当たった時のカード(`ErrorCopy.planLimitResourceName`)と同じ言い方。
@MainActor
enum UsageCopy {
    static var sectionTitle: String { L10n.tr("使っている量") }

    static func rowTitle(_ resource: PlanLimit.Resource) -> String {
        switch resource {
        case .apps: L10n.pick(L10n.tr("作ったアプリ"), review: L10n.tr("用意した記録帳"))
        case .recordsPerApp: L10n.tr("このアプリの記録")
        case .photosPerApp: L10n.tr("このアプリの写真")
        case .other: L10n.tr("このアプリで使える量")
        }
    }

    /// 「3 / 100」。数えない時は「3(上限なし)」
    static func amount(_ meter: UsageMeter) -> String {
        guard let limit = meter.limit else { return L10n.tr("\(meter.used)(上限なし)") }
        return "\(meter.used) / \(limit)"
    }

    /// VoiceOver(「/」を読ませない)
    static func accessibilityAmount(_ meter: UsageMeter) -> String {
        guard let limit = meter.limit else { return L10n.tr("\(meter.used)、上限なし") }
        return L10n.tr("\(limit) のうち \(meter.used)")
    }

    static func footer(isPro: Bool) -> String {
        #if APP_STORE_REVIEW
        return L10n.tr("使わないものを消すと、空きが増えます")
        #else
        return isPro ? L10n.tr("Pro プランの上限です") : L10n.tr("無料プランの上限です。使わないものを消すか、Pro にすると増やせます")
        #endif
    }

    /// 1 行のお知らせ
    static func notice(_ notice: UsageNotice) -> String {
        let what = ErrorCopy.planLimitResourceName(notice.resource)
        let amount = amount(notice.meter)
        if notice.meter.isAtLimit { return L10n.tr("\(what)が上限に達しています(\(amount))") }
        return L10n.tr("\(what)が上限に近づいています(\(amount))")
    }

    /// 「Pro にする」を出すか。作成者で、Pro にすると上がる時だけ(参加者のお知らせはそもそも作らない)。審査用の版は課金を持たない
    static func showsProButton(_ notice: UsageNotice, isPro: Bool) -> Bool {
        #if APP_STORE_REVIEW
        return false
        #else
        return notice.upgrade && !isPro
        #endif
    }
}
