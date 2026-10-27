import Foundation

/// 生成で新しく「誰でも見られる」記録に入った個人の情報の欄 1 つ(#253・openapi `GenerateResponse.meta.publicPersonalData[]`)。
public struct PublicPersonalField: Codable, Equatable, Sendable {
    /// entity id
    public var entity: String
    public var entityName: String
    /// field id
    public var field: String
    /// 表示名(ダイアログにそのまま出す)
    public var fieldName: String
    /// name | phone | address | email | birthday | location(未知の値もそのまま持つ)
    public var kind: String

    public init(entity: String, entityName: String, field: String, fieldName: String, kind: String) {
        self.entity = entity
        self.entityName = entityName
        self.field = field
        self.fieldName = fieldName
        self.kind = kind
    }

    enum CodingKeys: String, CodingKey { case entity, entityName, field, fieldName, kind }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        entity = try c.decode(String.self, forKey: .entity)
        field = try c.decode(String.self, forKey: .field)
        entityName = try c.decodeIfPresent(String.self, forKey: .entityName) ?? entity
        fieldName = try c.decodeIfPresent(String.self, forKey: .fieldName) ?? field
        kind = try c.decodeIfPresent(String.self, forKey: .kind) ?? ""
    }
}

/// 生成直後の確認ダイアログ(#253)の中身と、ボタンで generate に送る修正の指示文。
/// 「この欄を消す」/「自分だけにする」は既存の修正(mode edit)の流れに乗せる。
public struct PublicDataPrompt: Identifiable, Equatable, Sendable {
    public let id = UUID()
    /// 出した時のアプリの版(別の版に変わったら指示を送らない)
    public var version: Int
    public var fields: [PublicPersonalField]

    /// 欄が無ければ nil(旧サーバーはフィールドを送らない = 出さない)。
    public init?(fields: [PublicPersonalField], version: Int) {
        guard !fields.isEmpty else { return nil }
        self.fields = fields
        self.version = version
    }

    /// ダイアログに出す欄の名前(重複を除き「・」でつなぐ。英語は ", ")。
    public func fieldNames(english: Bool = false) -> String {
        Self.unique(fields.map(\.fieldName)).joined(separator: english ? ", " : "・")
    }

    /// 「この欄を消す」の指示。例: 予約の「お名前」と「電話番号」の欄を消して
    public func removeInstruction(english: Bool = false) -> String {
        let parts = groups.map { group in
            english
                ? "the " + group.names.map { "\"\($0)\"" }.joined(separator: " and ")
                    + (group.names.count > 1 ? " fields" : " field") + " from \(group.entityName)"
                : "\(group.entityName)の" + group.names.map { "「\($0)」" }.joined(separator: "と") + "の欄"
        }
        return english ? "Remove " + parts.joined(separator: ", and ") : parts.joined(separator: "と、") + "を消して"
    }

    /// 「自分だけにする」の指示。backend の共有の語の表(share_intent.go の「自分だけ」/ "only me")で scope が remove になる。
    /// 例: 予約を自分だけの記録にして
    public func privateInstruction(english: Bool = false) -> String {
        let names = groups.map(\.entityName)
        return english
            ? "Keep " + names.joined(separator: " and ") + " to only me (make it private)"
            : names.joined(separator: "と") + "を自分だけの記録にして"
    }

    /// entity ごとの欄の名前(出てきた順・重複なし)。
    private var groups: [(entityName: String, names: [String])] {
        var order: [String] = []
        var byEntity: [String: (entityName: String, names: [String])] = [:]
        for f in fields {
            if byEntity[f.entity] == nil {
                order.append(f.entity)
                byEntity[f.entity] = (f.entityName, [])
            }
            if byEntity[f.entity]?.names.contains(f.fieldName) == false { byEntity[f.entity]?.names.append(f.fieldName) }
        }
        return order.compactMap { byEntity[$0] }
    }

    private static func unique(_ xs: [String]) -> [String] {
        var seen = Set<String>()
        return xs.filter { seen.insert($0).inserted }
    }

    public static func == (a: Self, b: Self) -> Bool { a.id == b.id }
}
