import Foundation

/// `aiButton.saveTo`: generate の list の 1 項目 = entity の 1 件(field に入れる)。
/// 検証は backend `dsl/ai_save_to.go`。保存は利用者が押した時だけ(「全部保存」か 1 つずつ)。
public struct AISaveTo: Decodable, Sendable, Equatable {
    public var entity: String
    public var field: String

    public init(entity: String, field: String) {
        self.entity = entity
        self.field = field
    }

    /// 1 項目の記録の data: field に項目(上限で切る)、ほかの field は新しいフォームと同じ既定値。
    /// field が entity の string / text でなければ nil(古い DSL・検証を通らない形)。
    public func record(_ item: String, in entity: Entity) -> [String: JSONValue]? {
        guard entity.id == self.entity,
              let f = entity.storedFields.first(where: { $0.id == field && ($0.type == .string || $0.type == .text) })
        else { return nil }
        let text = item.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !text.isEmpty else { return nil }
        let limit = RecordImport.characterLimit(f)
        var data: [String: JSONValue] = [f.id: .string(String(String.UnicodeScalarView(text.unicodeScalars.prefix(limit))))]
        for x in entity.storedFields where data[x.id] == nil {
            let d = FieldCoding.defaultValue(for: x)
            if !d.isNull { data[x.id] = d }
        }
        return data
    }

    /// 今回足せる件数: entity の上限(`RecordImport.maxRecordsPerEntity`)までの残り。
    public static func room(existing: Int, wanted: Int) -> Int {
        max(0, min(wanted, RecordImport.maxRecordsPerEntity - existing))
    }
}
