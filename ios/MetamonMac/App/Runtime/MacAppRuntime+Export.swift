import AppKit
import UniformTypeIdentifiers
import MetamonDSL
import MetamonRenderer

/// `export` アクション(#421 の Mac 版・#1143): 記録を CSV / PDF にして、保存パネルで選んだ場所に保存する。
/// 表は iOS と同じ `makeExportFile`(Renderer)で作る。一時フォルダの写しは保存の後に消す(記録の写しを残さない)。
extension MacAppRuntime {
    func export(_ action: Action, _ ctx: ActionContext) async {
        guard let entityId = action.entity, let entity = dsl.entity(entityId) else {
            toast = L10n.tr("書き出す記録が見つかりません")
            return
        }
        // 参照先の名前(ref の見出し)も表に出すので一緒に読む(iOS と同じ)
        let refs = entity.fields.compactMap(\.refEntity)
        await load(entities: Set([entityId] + refs))
        let file: ExportFile
        do {
            file = try makeExportFile(action, screen: ctx.route.screen)
        } catch {
            toast = L10n.tr("書き出せませんでした")
            return
        }
        defer { ExportFiles.removeAll() }
        guard let destination = await MacExportSaver.chooseDestination(file) else { return } // キャンセル
        do {
            try MacExportSaver.save(file, to: destination)
            toast = L10n.tr("書き出しました")
        } catch {
            toast = L10n.tr("書き出せませんでした")
        }
    }
}

/// 保存パネルと、選んだ場所への書き込み。テストでは `chooseDestination` を差し替える
@MainActor
enum MacExportSaver {
    /// 保存先を聞く。nil ならキャンセル
    static var chooseDestination: @MainActor (ExportFile) async -> URL? = { file in
        let panel = NSSavePanel()
        panel.nameFieldStringValue = file.url.lastPathComponent
        panel.allowedContentTypes = contentTypes(file.url)
        panel.canCreateDirectories = true
        panel.isExtensionHidden = false
        let response: NSApplication.ModalResponse
        if let window = NSApp.keyWindow ?? NSApp.mainWindow {
            response = await panel.beginSheetModal(for: window)
        } else {
            response = panel.runModal()
        }
        return response == .OK ? panel.url : nil
    }

    static func contentTypes(_ url: URL) -> [UTType] {
        UTType(filenameExtension: url.pathExtension).map { [$0] } ?? []
    }

    /// 選んだ場所に置く(同じ名前があれば置き換える。置き換えるかはパネルが聞いている)
    static func save(_ file: ExportFile, to destination: URL) throws {
        let fm = FileManager.default
        if fm.fileExists(atPath: destination.path) { try fm.removeItem(at: destination) }
        try fm.copyItem(at: file.url, to: destination)
    }
}
