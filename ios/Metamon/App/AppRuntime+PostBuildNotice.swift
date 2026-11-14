import Foundation

/// 完成の直後のお知らせ(H7)。順番(`PostBuildNotice`)は Mac と共有する(`Shared/Runtime/PostBuildNotice.swift`・#968)。
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
