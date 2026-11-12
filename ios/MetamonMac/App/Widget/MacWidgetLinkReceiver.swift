import SwiftUI

/// デスクトップのウィジェット(#984)を押した時の `tailorit://widget?app=<appId>` を受ける。
/// ショートカットの「開く」(#1034)と同じ `IntentHost.request(.open)` に渡し、`MacRootView`(`macIntentRouting`)が一覧で選ぶ。
/// 通常の版は `MacLinkReceiver` の onOpenURL から `receive` を呼ぶ。審査用の版(共有のリンクを外す)だけこの modifier を付ける
struct MacWidgetLinkReceiver: ViewModifier {
    func body(content: Content) -> some View {
        content
            .onOpenURL { _ = Self.receive($0) }
            .handlesExternalEvents(preferring: ["*"], allowing: ["*"]) // 開いているウインドウで受ける(新しいウインドウを作らない)
    }

    /// ウィジェットの URL なら受けて true。appId が無い時は本体を前に出すだけ
    @MainActor
    @discardableResult
    static func receive(_ url: URL, host: IntentHost? = nil) -> Bool {
        guard MacWidgetLink.isWidget(url) else { return false }
        if let appId = MacWidgetLink.appId(from: url) {
            let host = host ?? .shared
            Task { await host.request(.open(appId: appId)) }
        }
        return true
    }
}
