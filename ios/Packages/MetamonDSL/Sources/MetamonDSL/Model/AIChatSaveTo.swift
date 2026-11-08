import Foundation

/// `aiChat.saveTo`: 話しかけて記録する(「今日カレー食べた」→ 食事の記録の案)。検証は backend `dsl/ai_chat_save_to.go`。
/// 依頼に field の形(`AITextSaveTo`)を付けると、サーバーが返事と一緒に `suggest`(field id → 値・型を確かめ済み)を返す。
/// 保存は利用者が「追加」を押した時だけ(`record` の data を通常の `createRecord` で)。
public struct AIChatSaveTo: Decodable, Sendable, Equatable {
    public var entity: String
    public var fields: [String]

    public init(entity: String, fields: [String]) {
        self.entity = entity
        self.fields = fields
    }

    /// backend `dsl.MaxAIChatSaveFields` / `AIChatSaveFieldTypes`。
    public static let maxFields = 8
    public static let fieldTypes: Set<FieldType> = [.string, .text, .number, .date, .enum]
    /// backend の suggest の上限(数 ±1e9)。
    static let maxNumber = 1e9

    /// 案に使う field(DSL の順)。entity が違う・使えない field が 1 つでもあれば nil(古い・検証を通らない DSL)。
    public func saveFields(in entity: Entity) -> [Field]? {
        guard entity.id == self.entity, !fields.isEmpty, fields.count <= Self.maxFields else { return nil }
        var out: [Field] = []
        for id in fields {
            guard let f = entity.storedFields.first(where: { $0.id == id }), Self.fieldTypes.contains(f.type) else { return nil }
            out.append(f)
        }
        return out
    }

    /// 依頼に付ける saveTo(backend `chatSaveToOf` と同じ値: 共有のアプリで作成者が払う時はこれが DSL と一致する)。
    public func request(in entity: Entity, today: Date = Date()) -> AITextSaveTo? {
        guard let fs = saveFields(in: entity) else { return nil }
        func trim(_ s: String?) -> String { (s ?? "").trimmingCharacters(in: .whitespacesAndNewlines) }
        let unit: (Field) -> String? = { f in trim(f.unit).isEmpty ? nil : trim(f.unit) }
        return AITextSaveTo(
            entity: entity.id, name: trim(entity.name).isEmpty ? nil : trim(entity.name),
            today: FieldCoding.dateFormatter.string(from: today),
            fields: fs.map { f in
                AITextSaveField(id: f.id, name: trim(f.name), type: f.type.rawValue,
                                options: f.type == .enum ? f.options : nil, unit: unit(f),
                                required: (f.required == true && f.defaultValue == nil) ? true : nil)
            })
    }

    /// 案の値のうち field に合うもの(サーバーと同じ規則で確かめ直す)。field の順。
    public func values(_ suggest: [String: JSONValue], in entity: Entity) -> [(field: Field, value: JSONValue)] {
        guard let fs = saveFields(in: entity) else { return [] }
        var out: [(field: Field, value: JSONValue)] = []
        for f in fs {
            if let v = suggest[f.id].flatMap({ Self.check($0, for: f) }) { out.append((f, v)) }
        }
        return out
    }

    /// 「追加」で保存する data: 案の値 + ほかの field は新しいフォームと同じ既定値。
    /// 何も合わない・required の field が空になる時は nil(カードを出さない)。
    public func record(_ suggest: [String: JSONValue], in entity: Entity) -> [String: JSONValue]? {
        let got = values(suggest, in: entity)
        guard !got.isEmpty else { return nil }
        var data: [String: JSONValue] = [:]
        for (f, v) in got { data[f.id] = v }
        for x in entity.storedFields where data[x.id] == nil {
            let d = FieldCoding.defaultValue(for: x)
            if !d.isNull { data[x.id] = d }
        }
        for x in entity.storedFields where x.required == true && !x.isComputed {
            guard let v = data[x.id], !v.isNull, v.stringValue?.isEmpty != true else { return nil }
        }
        return data
    }

    static func check(_ v: JSONValue, for f: Field) -> JSONValue? {
        switch f.type {
        case .number:
            guard let n = v.doubleValue, n.isFinite, abs(n) <= maxNumber else { return nil }
            return .number(n)
        case .enum:
            guard let s = v.stringValue, (f.options ?? []).contains(s) else { return nil }
            return .string(s)
        case .date:
            guard let s = v.stringValue, FieldCoding.dateFormatter.date(from: s) != nil else { return nil }
            return .string(s)
        case .string, .text:
            let s = (v.stringValue ?? "").trimmingCharacters(in: .whitespacesAndNewlines)
            guard !s.isEmpty else { return nil }
            return .string(String(String.UnicodeScalarView(s.unicodeScalars.prefix(RecordImport.characterLimit(f)))))
        default:
            return nil
        }
    }
}

/// `POST /v1/text` の `saveTo`(task chat だけ・openapi `TextRequest.saveTo`)。
public struct AITextSaveTo: Codable, Equatable, Sendable {
    public var entity: String
    public var name: String?
    /// 端末の今日(YYYY-MM-DD)。「今日」「昨日」の日付に使う
    public var today: String
    public var fields: [AITextSaveField]

    public init(entity: String, name: String?, today: String, fields: [AITextSaveField]) {
        self.entity = entity
        self.name = name
        self.today = today
        self.fields = fields
    }
}

public struct AITextSaveField: Codable, Equatable, Sendable {
    public var id: String
    public var name: String
    /// string | text | number | date | enum
    public var type: String
    public var options: [String]?
    public var unit: String?
    /// required で defaultValue の無い field(nil = false)
    public var required: Bool?

    public init(id: String, name: String, type: String, options: [String]? = nil, unit: String? = nil, required: Bool? = nil) {
        self.id = id
        self.name = name
        self.type = type
        self.options = options
        self.unit = unit
        self.required = required
    }
}
