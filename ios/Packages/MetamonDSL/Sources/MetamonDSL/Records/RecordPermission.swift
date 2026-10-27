import Foundation

/// 共有レコードを自分が変えられるか(#249 / #252)。appconfig の権限と同じ規則:
/// - private の entity は全部自分のもの
/// - shared / public は作った人(`Record.author`)だけが更新・削除できる。`edit:"members"` なら更新は誰でも、削除は作った人だけ
/// uid が分からない時と author が無い時(未送信の自分のレコード・旧データ)は止めない(サーバーが最後に判定する)。
public enum RecordPermission {
    public static func canEdit(_ record: Record, in entity: Entity?, uid: String?) -> Bool {
        isMine(record, in: entity, uid: uid) || (entity?.membersCanEdit ?? false)
    }

    public static func canDelete(_ record: Record, in entity: Entity?, uid: String?) -> Bool {
        isMine(record, in: entity, uid: uid)
    }

    private static func isMine(_ record: Record, in entity: Entity?, uid: String?) -> Bool {
        guard let entity, entity.isShared, let uid, let author = record.author else { return true }
        return author == uid
    }
}
