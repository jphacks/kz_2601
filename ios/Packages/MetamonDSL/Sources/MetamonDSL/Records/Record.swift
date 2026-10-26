import Foundation

/// 生成アプリのユーザーデータ1件(appconfig `Record`: {id, entity, data, author?, createdAt, updatedAt})。`data` のキーは Field.id。
public struct Record: Decodable, Identifiable, Hashable, Sendable {
    public var id: String
    public var data: [String: JSONValue]
    public var createdAt: String?
    public var updatedAt: String?
    /// 作った人の uid(shared / public の entity だけ backend が付ける・#249)。未送信の自分のレコードは nil。
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
