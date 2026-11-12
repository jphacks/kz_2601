import MetamonDSL

/// 記録のコピー・貼り付け(#1871)の結果を利用者に伝える。持たないホスト(プレビュー・テスト)では何も出さない。
@MainActor
public protocol RecordClipboardHost: AnyObject {
    /// 「記録をコピーしました」「3 項目を入れました」などの一言(画面下のトースト)。
    func notifyRecordClipboard(_ message: String)
}

public extension DSLRuntime {
    var recordClipboard: (any RecordClipboardHost)? { self as? any RecordClipboardHost }
}
