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
        baselineBeforeSave = baseline
        markClean()
        lastSaveResult = queuedOffline ? .queuedOffline : .saved
        saveCount += 1
    }

    /// 送った書き込みをサーバーが断った(上限・運営の停止・表示できない言葉など)。
    /// 「保存しました」を取り消し(保存バーは表示中でも消す)、入力は未保存に戻す(閉じる前に破棄の確認が出る)。
    /// `restoring` は送った入力。保存の後に空に戻したフォーム(タブ / ホームの根・#1723)なら、それを入力欄に戻す。
    /// 戻した後に入れ直した値があれば上書きしない(新しい入力を消さない)。
    public func markSaveRefused(restoring sent: [String: JSONValue]? = nil) {
        guard lastSaveResult != nil else { return }
        let untouched = !hasUnsavedChanges // 保存の後(空に戻した後)に入力していない
        if let before = baselineBeforeSave { baseline = before }
        baselineBeforeSave = nil
        lastSaveResult = nil
        guard let sent, !sent.isEmpty, untouched, payload != sent else { return }
        for f in entity.storedFields {
            values[f.id] = sent[f.id] ?? FieldCoding.defaultValue(for: f)
        }
    }

    /// 保存ボタンを押した後の表示。`savesBefore` は押す前の `saveCount`。
    /// 書き込みが無かった(必須が空・変更なし・確認待ち・権限なし・連打など)なら nil = 何も出さない。
    static func saveFeedback(savesBefore: Int?, form: FormDraft?) -> FormSaveResult? {
        guard let form, let savesBefore, form.saveCount != savesBefore else { return nil }
        return form.lastSaveResult
    }
}
