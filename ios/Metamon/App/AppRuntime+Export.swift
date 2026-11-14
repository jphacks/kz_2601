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
            let file = try makeExportFile(action, screen: ctx.route.screen)
            // 覚えたフォルダがあれば共有シートを出さずにそこへ(#2221)。無い / 書けない時は共有シート
            if writeToRememberedFolder(file) { return }
            exportFile = file
        } catch {
            toast = L10n.tr("書き出せませんでした")
        }
    }

    /// 覚えたフォルダ(iCloud Drive など)に書き出せたか(#2221)。
    /// 覚えていなければ false(1 回目は共有シートで選ぶ)。書き出せたら一時フォルダの写しは残さない。
    func writeToRememberedFolder(_ file: ExportFile, bookmark: FolderBookmark = .shared) -> Bool {
        guard bookmark.isRemembered(appId: document.appId) else { return false }
        do {
            _ = try bookmark.copy(file.url, appId: document.appId)
        } catch {
            return false // 消えた・許可が切れたフォルダ: 共有シートに戻す(覚えた物は残す)
        }
        ExportFiles.remove(file.url) // 記録の写しを一時フォルダに残さない
        toast = L10n.tr("保存しました")
        return true
    }

    /// 選んだフォルダを覚える(`FolderPicker` の直後)。次の書き出しからは選ばせない。
    func rememberExportFolder(_ folder: URL, bookmark: FolderBookmark = .shared) {
        do {
            try bookmark.remember(folder, appId: document.appId)
        } catch {
            toast = L10n.tr("書き出せませんでした")
        }
    }

    /// 覚えたフォルダを忘れる(選び直す時)。
    func forgetExportFolder(bookmark: FolderBookmark = .shared) {
        bookmark.forget(appId: document.appId)
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
