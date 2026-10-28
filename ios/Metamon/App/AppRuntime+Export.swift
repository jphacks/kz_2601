import SwiftUI
import UIKit
import MetamonDSL
import MetamonRenderer

/// `export` アクション(#421): 記録を CSV / PDF にして共有シート(ファイル・メール・AirDrop)を開く。
extension AppRuntime {
    func export(_ action: Action, _ ctx: ActionContext) async {
        guard let entityId = action.entity, let entity = dsl.entity(entityId) else {
            toast = L10n.tr("書き出す記録が見つかりません")
            return
        }
        // 参照先の名前(ref の見出し)も表に出すので一緒に読む
        let refs = entity.fields.compactMap(\.refEntity)
        await load(entities: Set([entityId] + refs))
        do {
            exportFile = try makeExportFile(action, screen: ctx.route.screen)
        } catch {
            toast = L10n.tr("書き出せませんでした")
        }
    }
}

/// UIActivityViewController を SwiftUI のシートに載せる(ShareLink はボタン自身にしか付けられず、toolbar / action から開けないため)。
struct ActivityView: UIViewControllerRepresentable {
    let items: [Any]

    func makeUIViewController(context: Context) -> UIActivityViewController {
        UIActivityViewController(activityItems: items, applicationActivities: nil)
    }

    func updateUIViewController(_ controller: UIActivityViewController, context: Context) {}
}
