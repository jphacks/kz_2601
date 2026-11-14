import Foundation

/// 完成の直後に出るお知らせ(H7)。同時に出さず、優先順位の高いものから 1 つずつ(前のが閉じたら次)出す。
/// iOS は各ホスト(`RootView` の要約・`PublicDataPromptHost`・`RetentionPromptHost`・`TutorialHost`・`AccountLinkPromptHost`)が
/// `AppRuntime.activeNotice` が自分の番の時だけ出す。Mac は `MacPostBuildNotices`(#968)が同じ順番で出す。
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
