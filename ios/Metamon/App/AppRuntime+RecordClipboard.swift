import MetamonRenderer

/// 記録のコピー・貼り付け(#1871)の結果は、ほかの一言と同じ画面下のトーストで出す。
extension AppRuntime: RecordClipboardHost {
    func notifyRecordClipboard(_ message: String) {
        toast = message
    }
}
