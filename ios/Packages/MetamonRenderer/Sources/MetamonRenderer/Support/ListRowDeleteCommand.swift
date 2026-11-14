import MetamonDSL

/// 一覧の ⌫(Mac): どの行を消すか。⌘C(`ListRowCopyCommand`)と同じくフォーカスのある行だけで、
/// 入力欄が文字を受けている時は入力欄の ⌫ に任せる。消せない行(他人の共有の記録など)は何もしない。
enum ListRowDeleteCommand {
    static func target(focusedRowId: String?, textInputActive: Bool, rows: [Record], deletable: (Record) -> Bool) -> Record? {
        guard let row = ListRowCopyCommand.target(focusedRowId: focusedRowId, textInputActive: textInputActive, rows: rows),
              deletable(row) else { return nil }
        return row
    }

    /// 確認の文。行の見出しが空なら `fallback`(entity の名前)。
    static func confirmMessage(title: String, fallback: String) -> String {
        let name = title.trimmingCharacters(in: .whitespacesAndNewlines)
        return L("「\(name.isEmpty ? fallback : name)」を削除しますか?")
    }
}
