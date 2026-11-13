import UIKit
import MetamonServices

/// ホーム画面のアイコンの長押しメニュー(#2232)。最近開いた生成アプリ 3 つ + 「声で頼む」。
/// 押されたらウィジェットのコントロールの「開く」と同じ道(`IntentHost.request(.control)`)を通す:
/// 読み込み前に来たら読み終えてから開く・消したアプリなら一覧を開く。審査用の版でもコントロールと同じく出す。
@MainActor
enum HomeQuickActions {
    static let openType = "app.tailorit.quick.open"
    static let talkType = "app.tailorit.quick.talk"
    static let appIdKey = "appId"

    static var recent = RecentApps()

    /// 開いたアプリを最近の先頭に(`AppRuntime.apply`)。メニューは次の `update` で描き直す。
    static func noteOpened(_ appId: String) {
        recent.noteOpened(appId)
    }

    /// 一覧が変わった(切替・削除・名前の変更)後に呼ぶ(`IntentHost.appsMayHaveChanged`)。
    static func update(apps: [AppSummary]) {
        let ids = Set(apps.map(\.id))
        for id in recent.ids where !ids.contains(id) && !apps.isEmpty { recent.forget(id) } // 消したアプリは覚えておかない
        UIApplication.shared.shortcutItems = QuickActionItem.items(recent: recent.ids, apps: apps).map(shortcut)
    }

    /// ログアウト(`wipeLocalData`): 前の人のアプリ名を残さない。
    static func clear() {
        UIApplication.shared.shortcutItems = []
    }

    static func shortcut(_ item: QuickActionItem) -> UIApplicationShortcutItem {
        switch item {
        case .app(let id, let name):
            UIApplicationShortcutItem(type: openType, localizedTitle: name, localizedSubtitle: nil,
                                      icon: UIApplicationShortcutIcon(systemImageName: "square.grid.2x2"),
                                      userInfo: [appIdKey: id as NSString])
        case .talk:
            UIApplicationShortcutItem(type: talkType, localizedTitle: L10n.tr("声で頼む"), localizedSubtitle: nil,
                                      icon: UIApplicationShortcutIcon(systemImageName: "mic.fill"), userInfo: nil)
        }
    }

    /// 押された項目 → 本体への頼み。知らない種類は nil。
    static func request(for type: String, userInfo: [String: NSSecureCoding]?) -> IntentHost.Pending? {
        switch type {
        case openType:
            guard let id = userInfo?[appIdKey] as? String, !id.isEmpty else { return nil }
            return .control(appId: id, entityId: nil)
        case talkType:
            return .talk
        default:
            return nil
        }
    }

    @discardableResult
    static func perform(_ item: UIApplicationShortcutItem) -> Bool {
        guard let pending = request(for: item.type, userInfo: item.userInfo) else { return false }
        Task { await IntentHost.shared.request(pending) }
        return true
    }
}

/// 長押しメニューを受けるシーンの代理。SwiftUI のシーンに足すだけ(画面は SwiftUI が作る)。
final class QuickActionSceneDelegate: NSObject, UIWindowSceneDelegate {
    /// 長押しメニューから起動した時(アプリが動いていなかった)
    func scene(_ scene: UIScene, willConnectTo session: UISceneSession, options connectionOptions: UIScene.ConnectionOptions) {
        guard let item = connectionOptions.shortcutItem else { return }
        Task { @MainActor in HomeQuickActions.perform(item) }
    }

    /// 動いている時に押された
    func windowScene(_ windowScene: UIWindowScene, performActionFor shortcutItem: UIApplicationShortcutItem) async -> Bool {
        await HomeQuickActions.perform(shortcutItem)
    }
}

/// シーンに `QuickActionSceneDelegate` を付ける。通常の版は `PushAppDelegate` がこれを継ぐ(#269)。
class MetamonAppDelegate: NSObject, UIApplicationDelegate {
    func application(_ application: UIApplication, configurationForConnecting connectingSceneSession: UISceneSession,
                     options: UIScene.ConnectionOptions) -> UISceneConfiguration {
        let config = UISceneConfiguration(name: nil, sessionRole: connectingSceneSession.role)
        config.delegateClass = QuickActionSceneDelegate.self
        return config
    }
}
