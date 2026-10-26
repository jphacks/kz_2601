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
