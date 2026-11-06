import Foundation

/// 完成の直後に出るお知らせ(H7)。同時に出さず、優先順位の高いものから 1 つずつ(前のが閉じたら次)出す。
/// 各ホスト(`RootView` の要約・`PublicDataPromptHost`・`RetentionPromptHost`・`TutorialHost`・`AccountLinkPromptHost`)は
/// `AppRuntime.activeNotice` が自分の番の時だけ出す。
enum PostBuildNotice: Int, CaseIterable, Comparable {
    /// 共有の記録が入った版の課金画面(買うかどうかで版が変わるので最初)
    case paywall
    /// 要約バナー(「元に戻す」付き)。「できた」をまず見せる
    case summary
    /// 公開の記録に個人の情報が入った確認(#253)
    case publicData
    /// 記録の保存期間の確認(#482)
    case retention
    /// 「こう話すと直せます」(#274)
    case editHint
    /// 引き継ぎ(匿名 → Apple)の案内(#276)。最初のアプリの直後には出さず、次の起動で出す
    case accountLink

    static func < (a: PostBuildNotice, b: PostBuildNotice) -> Bool { a.rawValue < b.rawValue }

    /// 待っているお知らせのうち、いま出すもの(無ければ nil)。
    static func current(_ pending: Set<PostBuildNotice>) -> PostBuildNotice? { pending.min() }
}

extension AppRuntime {
    /// 出したい(出ている)お知らせ。
    var pendingNotices: Set<PostBuildNotice> {
        var pending: Set<PostBuildNotice> = []
        #if !APP_STORE_REVIEW
        if paidGatePending || proGate.paywall?.placement == .root { pending.insert(.paywall) }
        #endif
        if summaryBanner != nil { pending.insert(.summary) }
        if publicDataPrompt != nil { pending.insert(.publicData) }
        if retentionPrompt != nil { pending.insert(.retention) }
        if showFirstEditHint { pending.insert(.editHint) }
        if showLinkPrompt { pending.insert(.accountLink) }
        return pending
    }

    /// いま出してよいお知らせ。
    var activeNotice: PostBuildNotice? { PostBuildNotice.current(pendingNotices) }

    /// `notice` の番か(上の順番で、前のものが全部閉じている)。
    func isNoticeTurn(_ notice: PostBuildNotice) -> Bool { activeNotice == notice }
}

/// 確認ダイアログのホストが `task(id:)` に使う: 中身が替わった時と、自分の番になった・外れた時に出し直す。
struct PromptTurn<ID: Hashable>: Hashable {
    var id: ID?
    var turn: Bool
}
