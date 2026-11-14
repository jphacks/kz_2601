// 審査用ビルドは外から届くものの入口(MacLinkReceiver)を外すので、Handoff も出さない・受けない(iOS と同じ)
#if !APP_STORE_REVIEW
import SwiftUI
import MetamonRenderer

/// 届いた Handoff(#1851)。アプリは押された通知と同じ道(`MacNotificationRouter`)で一覧から選び、
/// 画面と記録はそのアプリのキャンバスのランタイムができてから(開いていれば今すぐ)`AppCanvas` が開く。
@MainActor
@Observable
final class MacHandoffInbox {
    static let shared = MacHandoffInbox()

    private(set) var pending = PendingHandoff()
    @ObservationIgnored private let openApp: @MainActor (String) -> Void

    init(openApp: @escaping @MainActor (String) -> Void = { MacNotificationRouter.shared.open(appId: $0) }) {
        self.openApp = openApp
    }

    /// Handoff の activity なら画面を保留してアプリを開き true。ほかの activity は false(触らない)
    @discardableResult
    func receive(_ activity: NSUserActivity, now: Date = .now) -> Bool {
        guard let target = HandoffTarget(activity: activity) else { return false }
        pending.receive(target, now: now)
        openApp(target.appId)
        return true
    }

    func take(appId: String, now: Date = .now) -> HandoffTarget? {
        pending.take(appId: appId, now: now)
    }
}

/// この Mac で発行したシェアリンク(`MacShareAppModel`)。Handoff の `webpageURL`(受ける端末にアプリが無い時にブラウザで開く)に使う
@MainActor
enum MacHandoffLinks {
    private(set) static var byApp: [String: URL] = [:]

    static func set(_ url: URL?, appId: String) { byApp[appId] = url }
}

extension View {
    /// 中央のアプリの今の画面を Handoff に出す(#1851)。サーバーにある自分のアプリだけ(同梱の例・端末だけのアプリは iPhone で開けない)
    func macHandoff(_ target: HandoffTarget?, appName: String) -> some View {
        userActivity(HandoffActivity.type, isActive: target != nil) { activity in
            guard let target else { return }
            MainActor.assumeIsolated {
                HandoffActivity.configure(activity, target: target, appName: appName,
                                          webpageURL: MacHandoffLinks.byApp[target.appId])
            }
        }
    }
}
#endif
