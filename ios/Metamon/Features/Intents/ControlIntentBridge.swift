import Foundation
import WidgetKit
import MetamonServices

/// コントロール(#1993)と本体の橋渡し。ウィジェット側は MetamonWidget/Controls/ControlIntentBridge.swift(何もしない)
enum ControlIntentBridge {
    /// 押された: 「開く」と同じく bootstrap の後に、選んだアプリ(と記録の入力画面)を開く
    @MainActor
    static func open(appId: String?, entityId: String?) async {
        await IntentHost.shared.request(.control(appId: appId, entityId: entityId))
    }

    /// 一覧を集めて書く。集めている間に取り消された(ログアウトで消した等)なら書かない。戻り値は書いたか
    @MainActor
    @discardableResult
    static func refreshCatalog(apps: @MainActor () async -> [AppSummary],
                               targets: @MainActor () async -> [RecordTargetEntity],
                               defaults: UserDefaults? = WidgetSnapshot.defaults) async -> Bool {
        let apps = await apps()
        let targets = await targets()
        guard !Task.isCancelled else { return false }
        return writeCatalog(apps: apps, targets: targets, defaults: defaults)
    }

    /// コントロールの設定で選べる一覧を App Group に書く。変わった時だけ書き、コントロールを読み直す。戻り値は書いたか
    @MainActor
    @discardableResult
    static func writeCatalog(apps: [AppSummary], targets: [RecordTargetEntity],
                             defaults: UserDefaults? = WidgetSnapshot.defaults) -> Bool {
        let catalog = ControlCatalog(
            apps: apps.map { ControlCatalog.App(id: $0.id, name: $0.name) },
            recordTargets: targets.map {
                ControlCatalog.RecordTarget(appId: $0.appId, entityId: $0.entityId, appName: $0.appName,
                                            entityName: $0.entityName, showsEntity: $0.showsEntity)
            }
        )
        guard catalog.save(to: defaults) else { return false }
        reloadControls()
        return true
    }

    /// コントロールの見出しを今の一覧で描き直す
    static func reloadControls() {
        if #available(iOS 18, *) { ControlCenter.shared.reloadAllControls() }
    }
}
