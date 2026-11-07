import MetamonDSL

// MARK: - 保存の結果(M10: 保存していない経路で「保存しました」を出さない)

/// 保存ボタンが押した後に見せる結果。
public enum FormSaveResult: Equatable, Sendable {
    /// 保存して送った(または送信待ちに積んだがオンライン)
    case saved
    /// 端末に保存したがオフラインのため送信待ち
    case queuedOffline
}

extension FormDraft {
    /// ホストが実際に書き込んだ後に呼ぶ。閉じても破棄の確認を出さず、保存バーに結果を伝える。
    public func markSaved(queuedOffline: Bool) {
        markClean()
        lastSaveResult = queuedOffline ? .queuedOffline : .saved
        saveCount += 1
    }

    /// 保存ボタンを押した後の表示。`savesBefore` は押す前の `saveCount`。
    /// 書き込みが無かった(必須が空・変更なし・確認待ち・権限なし・連打など)なら nil = 何も出さない。
    static func saveFeedback(savesBefore: Int?, form: FormDraft?) -> FormSaveResult? {
        guard let form, let savesBefore, form.saveCount != savesBefore else { return nil }
        return form.lastSaveResult
    }
}
