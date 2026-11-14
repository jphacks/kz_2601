import AppKit
import MetamonDSL
import MetamonRenderer

/// 記録のコピー・貼り付け(#1871 の Mac・#2251)。行の右クリックの「記録をコピー」・⌘C で載せた記録を、
/// 別の生成アプリのフォームで ⌘V すると合う項目に入れる(保存はしない)。中身の作り方と対応は iOS と同じ `RecordClipboard`。
/// 結果の一言(「3 項目を入れました: …」)はほかの一言と同じ中央の列のトースト。
extension MacAppRuntime: RecordClipboardHost {
    func notifyRecordClipboard(_ message: String) {
        toast = message
    }
}

extension MacAppRuntime {
    /// ⌘V で記録を貼る先: いちばん上の画面のフォーム(変えられて、貼れる項目がある)。無ければ nil
    var recordPasteTarget: FormDraft? {
        let top = NavigationResolver.top(navigationState, dsl: dsl)
        guard let form = openForms.form(at: top), !form.isReadOnly,
              !RecordClipboard.copyableFields(of: form.entity).isEmpty else { return nil }
        return form
    }

    /// 編集 > ペースト(⌘V)を記録の貼り付けにするか。入力欄が文字を受けている時・確認が出ている時は普通のペースト
    func pastesRecord(textInputActive: Bool, board: NSPasteboard = .general) -> Bool {
        MacRecordPaste.handles(textInputActive: textInputActive, dialogShown: isShowingDialog,
                               hasForm: recordPasteTarget != nil, hasRecord: RecordPasteboard.hasRecord(on: board))
    }

    /// ⌘V: クリップボードの記録をいちばん上のフォームに入れる
    func pasteRecordFromMenu() {
        guard let form = recordPasteTarget else { return }
        RecordPasteboard.paste(into: form, rt: self)
    }
}

/// ⌘V を記録の貼り付けにするかの判定(#2251)。
enum MacRecordPaste {
    /// 入力欄にフォーカスが無く、確認が出ておらず、貼れるフォームがあり、記録が載っている時だけ
    static func handles(textInputActive: Bool, dialogShown: Bool, hasForm: Bool, hasRecord: Bool) -> Bool {
        !textInputActive && !dialogShown && hasForm && hasRecord
    }

    /// 入力欄(フィールドエディタ)が文字を受けているか
    @MainActor
    static var textInputActive: Bool {
        NSApp?.keyWindow?.firstResponder is NSText
    }
}
