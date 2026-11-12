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

    /// コントロールの設定で選べる一覧を App Group に書く。変わったらコントロールを読み直す
    @MainActor
    static func writeCatalog(apps: [AppSummary], targets: [RecordTargetEntity]) {
        let catalog = ControlCatalog(
            apps: apps.map { ControlCatalog.App(id: $0.id, name: $0.name) },
            recordTargets: targets.map {
                ControlCatalog.RecordTarget(appId: $0.appId, entityId: $0.entityId, appName: $0.appName,
                                            entityName: $0.entityName, showsEntity: $0.showsEntity)
            }
        )
        guard catalog.save() else { return }
        if #available(iOS 18, *) { ControlCenter.shared.reloadAllControls() }
    }
}
