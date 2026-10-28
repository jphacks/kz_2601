import MetamonDSL
import MetamonRenderer

/// `chatThread` のメッセージ送信(#260)。共有アプリへの初めての投稿なら、フォームの saveRecord と同じく
/// 先に「投稿に表示する名前」を聞き(#250)、答えたら保存する。保存の経路は `createRecord`(ローカル DB → 同期キュー)。
extension AppRuntime: RecordPosting {
    func postRecord(entity: String, data: [String: JSONValue]) async -> Bool {
        guard dsl.entity(entity) != nil else { return false }
        if postAuthor(for: entity) != nil, !AuthorProfile.asked {
            authorNamePrompt = AuthorNamePrompt { [weak self] in _ = await self?.createRecord(entity: entity, data: data) }
            return true
        }
        return await createRecord(entity: entity, data: data)
    }
}
