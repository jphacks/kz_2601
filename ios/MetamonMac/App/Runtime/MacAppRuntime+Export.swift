import AppKit
import PDFKit
import UniformTypeIdentifiers
import MetamonDSL
import MetamonRenderer

/// `export` を Mac でどう出すか。iOS の共有シート(保存・共有・プリントが 1 つに並ぶ)の代わりに最初に選ぶ(#946・#953)
enum MacExportOutput: Equatable {
    case save, share, print
}

/// `export` アクション(#421 の Mac 版・#1143): 記録を CSV / PDF にして、保存パネル・共有メニュー・プリントで出す。
/// 表は iOS と同じ `makeExportFile`(Renderer)で、書き出しごとのフォルダに作る(ほかのウインドウの共有・行のドラッグの写しを消さない)。
/// 保存は終わったら自分のフォルダだけ消す(記録の写しを残さない)。プリントは一時ファイルを作らない。
/// 共有は送り終えた・失敗した・閉じた時に自分のフォルダを消す(`MacExportShareSession`)。残ったものは起動時と終了時に片付ける。
extension MacAppRuntime {
    func export(_ action: Action, _ ctx: ActionContext) async {
        guard let entityId = action.entity, let entity = dsl.entity(entityId) else {
            toast = L10n.tr("書き出す記録が見つかりません")
            return
        }
        // 参照先の名前(ref の見出し)も表に出すので一緒に読む(iOS と同じ)
        let refs = entity.fields.compactMap(\.refEntity)
        await load(entities: Set([entityId] + refs))
        guard let output = await MacExportSaver.chooseOutput() else { return } // キャンセル
        switch output {
        case .save: await saveExport(action, ctx)
        case .share: shareExport(action, ctx)
        case .print: printExport(action, ctx)
        }
    }

    private func saveExport(_ action: Action, _ ctx: ActionContext) async {
        let file: ExportFile
        do {
            file = try makeExportFile(action, screen: ctx.route.screen, clearingPrevious: false)
        } catch {
            toast = L10n.tr("書き出せませんでした")
            return
        }
        defer { ExportFiles.remove(file.url) }
        guard let destination = await MacExportSaver.chooseDestination(file) else { return } // キャンセル
        do {
            try MacExportSaver.save(file, to: destination)
            toast = L10n.tr("書き出しました")
        } catch {
            toast = L10n.tr("書き出せませんでした")
        }
    }

    /// 共有メニュー(NSSharingServicePicker)。CSV / PDF のファイルをそのまま渡す
    private func shareExport(_ action: Action, _ ctx: ActionContext) {
        guard let file = try? makeExportFile(action, screen: ctx.route.screen, clearingPrevious: false) else {
            toast = L10n.tr("書き出せませんでした")
            return
        }
        if !MacExportSaver.share(file.url) {
            ExportFiles.remove(file.url)
            toast = L10n.tr("書き出せませんでした")
        }
    }

    /// プリント。CSV を選んだボタンでも、紙には PDF と同じ表(グラフ付き)を出す
    private func printExport(_ action: Action, _ ctx: ActionContext) {
        guard let table = try? exportTable(action),
              let document = PDFDocument(data: ExportPDF.data(table, charts: exportChartImages(screen: ctx.route.screen,
                                                                                               entity: action.entity ?? ""))),
              MacExportSaver.print(document) else {
            toast = L10n.tr("プリントできませんでした")
            return
        }
    }
}

/// 出し方の選択・保存パネル・共有メニュー・プリントと、選んだ場所への書き込み。テストでは各 `static var` を差し替える
@MainActor
enum MacExportSaver {
    /// 保存・共有・プリントのどれで出すかを聞く。nil ならキャンセル
    static var chooseOutput: @MainActor () async -> MacExportOutput? = {
        let alert = NSAlert()
        alert.messageText = L10n.tr("書き出し方を選んでください")
        alert.addButton(withTitle: L10n.tr("保存…")).toolTip = L10n.tr("CSV / PDF をファイルに保存します")
        alert.addButton(withTitle: L10n.tr("共有…")).toolTip = L10n.tr("メール・メッセージ・AirDrop などに送ります")
        alert.addButton(withTitle: L10n.tr("プリント…")).toolTip = L10n.tr("一覧をプリントします")
        let cancel = alert.addButton(withTitle: L10n.tr("キャンセル"))
        cancel.keyEquivalent = "\u{1b}" // Esc
        let response: NSApplication.ModalResponse
        if let window = NSApp.keyWindow ?? NSApp.mainWindow {
            response = await alert.beginSheetModal(for: window)
        } else {
            response = alert.runModal()
        }
        return output(for: response)
    }

    /// アラートのボタンの順(保存・共有・プリント・キャンセル)から選んだ出し方
    static func output(for response: NSApplication.ModalResponse) -> MacExportOutput? {
        switch response {
        case .alertFirstButtonReturn: .save
        case .alertSecondButtonReturn: .share
        case .alertThirdButtonReturn: .print
        default: nil
        }
    }

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

    /// 共有メニューを前のウインドウの中央に出す。出せなければ false。
    /// 送り終えた・失敗した・閉じた時に写しを消す(`MacExportShareSession`)
    static var share: @MainActor (URL) -> Bool = { url in
        guard let view = (NSApp.keyWindow ?? NSApp.mainWindow)?.contentView else { return false }
        let anchor = NSRect(x: view.bounds.midX, y: view.bounds.midY, width: 1, height: 1)
        MacExportShareSession(url: url).show(relativeTo: anchor, of: view)
        return true
    }

    /// プリントのパネルを出す(ウインドウがあればシートで)。出せなければ false
    static var print: @MainActor (PDFDocument) -> Bool = { document in
        guard let info = NSPrintInfo.shared.copy() as? NSPrintInfo,
              let operation = document.printOperation(for: info, scalingMode: .pageScaleToFit, autoRotate: true)
        else { return false }
        if let window = NSApp.keyWindow ?? NSApp.mainWindow {
            operation.runModal(for: window, delegate: nil, didRun: nil, contextInfo: nil)
        } else {
            operation.run()
        }
        return true
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

/// 共有メニュー 1 回ぶん。送り終えた・失敗した・選ばずに閉じた時に、渡した写し(書き出しのフォルダ)を消す。
/// メニューの delegate は弱参照なので、終わるまで `active` で持つ
@MainActor
final class MacExportShareSession: NSObject, NSSharingServicePickerDelegate, NSSharingServiceDelegate {
    private static var active: Set<MacExportShareSession> = []
    static var activeCount: Int { active.count }

    let url: URL
    private var picker: NSSharingServicePicker?

    init(url: URL) {
        self.url = url
        super.init()
        Self.active.insert(self)
    }

    func show(relativeTo rect: NSRect, of view: NSView) {
        let picker = NSSharingServicePicker(items: [url])
        picker.delegate = self
        self.picker = picker
        picker.show(relativeTo: rect, of: view, preferredEdge: .minY)
    }

    /// 写しを消して手放す(2 回呼んでもよい)
    func finish() {
        guard Self.active.remove(self) != nil else { return }
        ExportFiles.remove(url)
        picker = nil
    }

    // MARK: NSSharingServicePickerDelegate

    func sharingServicePicker(_ picker: NSSharingServicePicker,
                              delegateFor sharingService: NSSharingService) -> NSSharingServiceDelegate? {
        self
    }

    func sharingServicePicker(_ picker: NSSharingServicePicker, didChoose service: NSSharingService?) {
        if service == nil { finish() } // 選ばずに閉じた
    }

    // MARK: NSSharingServiceDelegate

    func sharingService(_ sharingService: NSSharingService, didShareItems items: [Any]) {
        finish()
    }

    func sharingService(_ sharingService: NSSharingService, didFailToShareItems items: [Any], error: any Error) {
        finish()
    }
}
