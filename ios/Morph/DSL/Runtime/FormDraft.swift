import Foundation
import Observation

/// `screen.entity` を持つ画面が1件だけ持つドラフト。入力系 Leaf の `field` はこのキー。
/// recordId 付きで開かれたら既存レコードをロードして編集モード。
@MainActor
@Observable
final class FormDraft {
    let entity: Entity
    private(set) var recordId: String?
    var values: [String: JSONValue] = [:]

    init(entity: Entity, record: Record? = nil) {
        self.entity = entity
        reset()
        if let record { load(record) }
    }

    var isEditing: Bool { recordId != nil }

    func reset() {
        recordId = nil
        values = [:]
        for f in entity.fields {
            values[f.id] = FieldCoding.defaultValue(for: f)
        }
    }

    func load(_ record: Record) {
        recordId = record.id
        for f in entity.fields {
            values[f.id] = record.data[f.id] ?? FieldCoding.defaultValue(for: f)
        }
    }

    /// appconfig に送る `data`(Field.type に合わせて正規化・空は落とす)。
    var payload: [String: JSONValue] {
        var out: [String: JSONValue] = [:]
        for f in entity.fields {
            if let v = FieldCoding.normalize(values[f.id] ?? .null, for: f) { out[f.id] = v }
        }
        return out
    }

    /// required 未入力のフィールド名。
    var missingRequired: [String] {
        entity.fields.filter { ($0.required ?? false) && FieldCoding.normalize(values[$0.id] ?? .null, for: $0) == nil }.map(\.name)
    }
}

/// Field.type ごとの値表現(appconfig `RecordData` の規約と一致):
/// string/text→string, integer/currency→整数, number→数値, boolean→bool, date→"YYYY-MM-DD", datetime→RFC3339, enum→options の1つ,
/// photo→"data:image/jpeg;base64,..." or https URL, location→"lat,lng"。
enum FieldCoding {
    static let dateFormatter: DateFormatter = {
        let f = DateFormatter()
        f.locale = Locale(identifier: "en_US_POSIX")
        f.dateFormat = "yyyy-MM-dd"
        return f
    }()

    static let datetimeFormatter = ISO8601DateFormatter()

    static func defaultValue(for f: Field) -> JSONValue {
        if let d = f.defaultValue {
            if f.type.isDate, d.stringValue == "now" { return .string(encode(Date(), for: f)) }
            return d
        }
        switch f.type {
        case .boolean: return .bool(false)
        case .enum: return f.options?.first.map { .string($0) } ?? .null
        case .date, .datetime: return .string(encode(Date(), for: f))
        default: return .null
        }
    }

    static func encode(_ date: Date, for f: Field) -> String {
        f.type == .date ? dateFormatter.string(from: date) : datetimeFormatter.string(from: date)
    }

    static func date(from value: JSONValue) -> Date? {
        guard let s = value.stringValue else { return nil }
        return datetimeFormatter.date(from: s) ?? dateFormatter.date(from: s)
    }

    /// 送信前の正規化。空文字/未入力は nil(キーごと落とす)。
    static func normalize(_ v: JSONValue, for f: Field) -> JSONValue? {
        switch f.type {
        case .integer, .currency:
            if let n = v.doubleValue { return .number(n.rounded()) }
            if let s = v.stringValue, let n = Double(s.replacingOccurrences(of: ",", with: "")) { return .number(n.rounded()) }
            return nil
        case .number:
            if let n = v.doubleValue { return .number(n) }
            if let s = v.stringValue, let n = Double(s) { return .number(n) }
            return nil
        case .boolean:
            return .bool(v.truthy)
        default:
            if let s = v.stringValue { return s.isEmpty ? nil : .string(s) }
            return v.isNull ? nil : v
        }
    }

    /// 一覧・集計の表示用整形。
    static func display(_ v: JSONValue, field: Field?) -> String {
        guard let field else { return v.displayString }
        switch field.type {
        case .currency: return currency(v.doubleValue ?? 0)
        case .number, .integer:
            guard let n = v.doubleValue else { return "" }
            let s = n == n.rounded() ? String(Int(n)) : String(format: "%.1f", n)
            return field.unit.map { s + " " + $0 } ?? s
        case .date:
            return v.stringValue ?? ""
        case .datetime:
            guard let d = date(from: v) else { return v.displayString }
            return d.formatted(date: .numeric, time: .shortened)
        case .boolean: return v.truthy ? "はい" : "いいえ"
        case .photo: return v.isNull ? "" : "写真"
        default: return v.displayString
        }
    }

    static func currency(_ n: Double) -> String {
        let f = NumberFormatter()
        f.numberStyle = .decimal
        f.maximumFractionDigits = 0
        return "¥" + (f.string(from: NSNumber(value: n)) ?? "0")
    }
}
