import Foundation

/// 生成アプリのユーザーデータ1件(appconfig `Record`: {id, entity, data, author?, authorName?, createdAt, updatedAt})。`data` のキーは Field.id。
public struct Record: Decodable, Identifiable, Hashable, Sendable {
    public var id: String
    public var data: [String: JSONValue]
    public var createdAt: String?
    public var updatedAt: String?
    /// 作った人の uid(shared / public の entity だけ backend が付ける・#249)。未送信の自分のレコードは nil。
    public var author: String?
    /// 投稿者の表示名(shared / public・#250)。backend は未送信なら「匿名」を付ける。
    public var authorName: String?
    /// 印の状態(#255・marks のある共有 entity だけ backend が付ける)。nil = サーバーが返していない
    public var marks: [String: MarkState]?

    public init(id: String, data: [String: JSONValue], createdAt: String? = nil, updatedAt: String? = nil,
                author: String? = nil, authorName: String? = nil, marks: [String: MarkState]? = nil) {
        self.id = id
        self.data = data
        self.createdAt = createdAt
        self.updatedAt = updatedAt
        self.author = author
        self.authorName = authorName
        self.marks = marks
    }

    private enum Keys: String, CodingKey { case id, data, createdAt, updatedAt, author, authorName, marks }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: Keys.self)
        id = try c.decode(String.self, forKey: .id)
        data = try c.decodeIfPresent([String: JSONValue].self, forKey: .data) ?? [:]
        createdAt = try? c.decodeIfPresent(String.self, forKey: .createdAt)
        updatedAt = try? c.decodeIfPresent(String.self, forKey: .updatedAt)
        author = try? c.decodeIfPresent(String.self, forKey: .author)
        authorName = try? c.decodeIfPresent(String.self, forKey: .authorName)
        marks = try? c.decodeIfPresent([String: MarkState].self, forKey: .marks)
    }

    /// field の値。予約値 `$author` は表示名、`$createdAt` は作成日時(#250)、`$marks.<id>` は印の件数(#255)。
    public subscript(field: String) -> JSONValue {
        if let id = RecordValue.markId(field) { return .number(Double(markState(id).count)) }
        return switch field {
        case RecordValue.author: authorName.map(JSONValue.string) ?? .null
        case RecordValue.createdAt: createdAt.map(JSONValue.string) ?? .null
        default: data[field] ?? .null
        }
    }
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
