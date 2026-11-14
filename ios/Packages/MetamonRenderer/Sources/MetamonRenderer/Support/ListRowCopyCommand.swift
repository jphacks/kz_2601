import MetamonDSL

/// 一覧の ⌘C(Mac・#1332): どの行をコピーするか。
/// 行にフォーカスがあり、入力欄が文字を受けていない時だけその行。入力欄のときは入力欄の普通のコピーに任せる。
enum ListRowCopyCommand {
    static func target(focusedRowId: String?, textInputActive: Bool, rows: [Record]) -> Record? {
        guard !textInputActive, let id = focusedRowId else { return nil }
        return rows.first { $0.id == id }
    }
}

#if os(macOS)
import AppKit

extension ListRowCopyCommand {
    /// キーウインドウで入力欄(フィールドエディタ・テキストビュー)が文字を受けているか。
    @MainActor
    static var textInputActive: Bool {
        NSApp?.keyWindow?.firstResponder is NSText
    }
}
#endif
