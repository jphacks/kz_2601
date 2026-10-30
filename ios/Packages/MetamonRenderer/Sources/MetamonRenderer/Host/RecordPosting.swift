import MetamonDSL

/// 利用者が自分で書いて送る 1 件(`chatThread` のメッセージ・#260)。`createRecord` と違い、共有アプリへの
/// 初めての投稿ならホストが先に「投稿に表示する名前」を聞いてから保存する(フォームの saveRecord と同じ・#250)。
/// `DSLRuntime` 本体には足さず、対応するホストだけが準拠する(未対応のランタイムは `createRecord` に落ちる)。
@MainActor
public protocol RecordPosting: AnyObject {
    /// 送れたら(名前を聞いている間も含めて保存が決まったら)true。
    func postRecord(entity: String, data: [String: JSONValue]) async -> Bool
}

@MainActor
extension DSLRuntime {
    /// 利用者の投稿を 1 件保存する。`RecordPosting` なら名前の確認を挟み、そうでなければ `createRecord`。
    func post(entity: String, data: [String: JSONValue]) async -> Bool {
        if let poster = self as? RecordPosting { return await poster.postRecord(entity: entity, data: data) }
        return await createRecord(entity: entity, data: data)
    }
}
