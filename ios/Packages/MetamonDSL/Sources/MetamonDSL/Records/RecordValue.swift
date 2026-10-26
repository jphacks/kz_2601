import Foundation

/// field id の代わりに書ける、どのレコードにもある値(#250 #251)。正典は schema の `$defs/Entity` `x-recordValues`。
/// - `$author`: 投稿者の表示名(shared / public の entity だけ)。filter では投稿者の uid と比べ、値は `{"me": true}`
/// - `$createdAt`: レコードを作った日時
public enum RecordValue {
    public static let author = "$author"
    public static let createdAt = "$createdAt"
    public static let all: Set<String> = [author, createdAt]

    /// 表示・並べ替え用の仮の Field(一覧の書式と table の見出しに使う)。entity で使えない値は nil。
    static func field(_ id: String, in entity: Entity) -> Field? {
        switch id {
        case author where entity.isShared: Field(id: author, name: "投稿者", type: .string)
        case createdAt: Field(id: createdAt, name: "投稿日時", type: .datetime)
        default: nil
        }
    }
}

public extension Filter {
    /// `{"me": true}` = サインイン中の人(field `$author` 専用)。
    static let meValue: JSONValue = .object(["me": .bool(true)])

    var isMe: Bool { value == Self.meValue }

    /// `{"me": true}` を uid に置き換える。uid が分からない時は空文字(= 作者の無い自分の未送信レコードだけに一致)。
    func resolvingMe(uid: String?) -> Filter {
        isMe ? Filter(field: field, op: op, value: .string(uid ?? "")) : self
    }
}
