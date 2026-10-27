import Foundation

/// 生成アプリのユーザーデータ1件(appconfig `Record`: {id, entity, data, author, createdAt, updatedAt})。`data` のキーは Field.id。
public struct Record: Decodable, Identifiable, Hashable, Sendable {
    public var id: String
    public var data: [String: JSONValue]
    public var createdAt: String?
    public var updatedAt: String?
    /// 投稿者の uid(public/shared entity でサーバーが付ける)。private entity・未送信の行は nil。
    public var author: String?

    public init(id: String, data: [String: JSONValue], createdAt: String? = nil, updatedAt: String? = nil, author: String? = nil) {
        self.id = id
        self.data = data
        self.createdAt = createdAt
        self.updatedAt = updatedAt
        self.author = author
    }

    private enum Keys: String, CodingKey { case id, data, createdAt, updatedAt, author }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: Keys.self)
        id = try c.decode(String.self, forKey: .id)
        data = try c.decodeIfPresent([String: JSONValue].self, forKey: .data) ?? [:]
        createdAt = try? c.decodeIfPresent(String.self, forKey: .createdAt)
        updatedAt = try? c.decodeIfPresent(String.self, forKey: .updatedAt)
        author = try? c.decodeIfPresent(String.self, forKey: .author)
    }

    public subscript(field: String) -> JSONValue { data[field] ?? .null }
}

/// `Field.hidden`(#262・クイズの正解)を誰に見せるか。値は端末に届いているので画面に出さないだけ(厳密な秘匿ではない)。
public enum HiddenFields {
    /// フォームで hidden の入力を出してよいか。
    /// - 新規(record 無し): 入力する本人が作者になるので出す
    /// - author 無し: private entity / 未送信の自分の行なので出す
    /// - author 有り: 自分の uid と一致する時だけ出す(自分の uid が分からなければ出さない)
    public static func canEdit(record: Record?, currentUid: String?) -> Bool {
        guard let author = record?.author, !author.isEmpty else { return true }
        guard let currentUid, !currentUid.isEmpty else { return false }
        return author == currentUid
    }

    /// 表示用レコードから hidden の値を落とす(一覧・表・グラフ・カレンダー等)。式の評価は元のレコードで行うので採点は動く。
    public static func redact(_ records: [Record], in entity: Entity?) -> [Record] {
        guard let hidden = entity?.hiddenFieldIds, !hidden.isEmpty else { return records }
        return records.map { r in
            var out = r
            for id in hidden { out.data.removeValue(forKey: id) }
            return out
        }
    }

    /// hidden の値を落とした `data`(フォームの値を端末内 AI へ渡す時など)。
    public static func redact(_ data: [String: JSONValue], in entity: Entity?) -> [String: JSONValue] {
        guard let hidden = entity?.hiddenFieldIds, !hidden.isEmpty else { return data }
        return data.filter { !hidden.contains($0.key) }
    }
}
