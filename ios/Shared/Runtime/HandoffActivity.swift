import Foundation
import MetamonDSL
import MetamonRenderer

/// 連係の Handoff(#1851): 今開いている生成アプリの画面を、同じ Apple アカウントのもう一方の端末(iPhone ⇄ Mac)で続きから開く。
/// 出す側も受ける側も iOS と Mac で同じ決まり。渡すのは appId・画面・記録の id だけ。
/// 記録の中身・題名・書きかけのフォームは渡さない(activity の題名もアプリの名前だけ)。検索・予測には出さない。
struct HandoffTarget: Equatable, Sendable {
    var appId: String
    var screenId: String?
    var recordId: String?

    /// 今いちばん上に見えている画面(モーダル → 選択中タブ / main の順)と、その画面の記録
    init(appId: String, navigation: NavigationState, dsl: AppDSL) {
        let top = NavigationResolver.top(navigation, dsl: dsl)
        self.init(appId: appId, screenId: top.screen.isEmpty ? nil : top.screen, recordId: top.recordId)
    }

    init(appId: String, screenId: String? = nil, recordId: String? = nil) {
        self.appId = appId
        self.screenId = screenId
        self.recordId = recordId
    }

    /// 届いた activity から読む。種類が違う・appId が無いものは nil(知らない activity は何もしない)
    init?(activity: NSUserActivity) {
        guard activity.activityType == HandoffActivity.type else { return nil }
        self.init(userInfo: activity.userInfo ?? [:])
    }

    init?(userInfo: [AnyHashable: Any]) {
        guard let appId = Self.text(userInfo[HandoffActivity.Key.appId]) else { return nil }
        self.init(appId: appId, screenId: Self.text(userInfo[HandoffActivity.Key.screenId]),
                  recordId: Self.text(userInfo[HandoffActivity.Key.recordId]))
    }

    private static func text(_ value: Any?) -> String? {
        guard let s = value as? String, !s.isEmpty else { return nil }
        return s
    }
}

enum HandoffActivity {
    /// 両方の Info.plist の `NSUserActivityTypes` にも書く
    static let type = "jp.kz2601.metamon.view"

    enum Key {
        static let appId = "appId"
        static let screenId = "screenId"
        static let recordId = "recordId"
    }

    /// SwiftUI の `.userActivity` が渡す activity に今の画面を書く。前の画面の記録の id が残らないよう userInfo は丸ごと置き換える。
    /// `webpageURL`: 受ける端末にアプリが無い時にブラウザで開く Universal Link(発行済みのシェアリンク)。無ければ nil
    @MainActor
    static func configure(_ activity: NSUserActivity, target: HandoffTarget, appName: String, webpageURL: URL? = nil) {
        activity.title = appName
        var info: [String: String] = [Key.appId: target.appId]
        if let screen = target.screenId { info[Key.screenId] = screen }
        if let record = target.recordId { info[Key.recordId] = record }
        activity.userInfo = info
        activity.requiredUserInfoKeys = [Key.appId]
        activity.webpageURL = webpageURL.flatMap { $0.scheme == "https" ? $0 : nil }
        activity.isEligibleForHandoff = true
        activity.isEligibleForSearch = false
        activity.isEligibleForPublicIndexing = false
        #if os(iOS)
        activity.isEligibleForPrediction = false
        #endif
        activity.needsSave = false
    }
}

/// 届いた Handoff の、アプリを開いた後に出す画面の保留。アプリの切り替えが終わる(Mac はキャンバスのランタイムができる)まで持つ。
/// 届いてから時間が経ったものは捨てる(開けなかったアプリを後で手で開いた時に、勝手に画面が動かないように)
struct PendingHandoff: Equatable {
    static let lifetime: TimeInterval = 60

    private(set) var target: HandoffTarget?
    private(set) var receivedAt: Date?

    mutating func receive(_ target: HandoffTarget, now: Date = .now) {
        self.target = target
        receivedAt = now
    }

    /// そのアプリ宛ての保留を渡して空にする。別のアプリ宛てなら nil(保留は残す)。古いものは捨てて nil
    mutating func take(appId: String, now: Date = .now) -> HandoffTarget? {
        guard let target, let receivedAt else { return nil }
        if now.timeIntervalSince(receivedAt) > Self.lifetime {
            clear()
            return nil
        }
        guard target.appId == appId else { return nil }
        clear()
        return target
    }

    mutating func clear() {
        target = nil
        receivedAt = nil
    }
}

extension RuntimeNavigating {
    /// 画面マップ(Mac #970)・Handoff(#1851)から画面を開く。モーダルを閉じ、根(タブ)から矢印をたどって開く(戻るで来た道を戻れる)。
    /// 無い画面は何もしない
    func reveal(screen: String, recordId: String? = nil) {
        guard let next = NavigationResolver.reveal(screen, recordId: recordId, graph: ScreenGraph(dsl: dsl), dsl: dsl,
                                                   from: navigationState) else { return }
        setNavigation(next)
    }

    /// 届いた Handoff の画面を開く。画面が無い(根の画面・消された画面)なら何もしない
    func reveal(_ target: HandoffTarget) {
        guard let screen = target.screenId else { return }
        reveal(screen: screen, recordId: target.recordId)
    }
}
