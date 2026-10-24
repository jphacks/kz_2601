import Foundation

/// Field.type ごとの値表現(appconfig `RecordData` の規約と一致):
/// string/text→string, integer/currency→整数, number→数値, boolean→bool, date→"YYYY-MM-DD", datetime→RFC3339, enum→options の1つ,
/// photo→"data:image/jpeg;base64,..." or https URL, location→"lat,lng"。
public enum FieldCoding {
    public static let dateFormatter: DateFormatter = {
        let f = DateFormatter()
        f.locale = Locale(identifier: "en_US_POSIX")
        f.dateFormat = "yyyy-MM-dd"
        return f
    }()

    public static let datetimeFormatter = ISO8601DateFormatter()

    public static func defaultValue(for f: Field) -> JSONValue {
        if let d = f.defaultValue {
            if f.type.isDate, d.stringValue == "now" { return .string(encode(Date(), for: f)) }
            return d
        }
        switch f.type {
        case .boolean: return .bool(false)
        case .enum: return f.options?.first.map { .string($0) } ?? .null
        case .ref: return .null
        case .date, .datetime: return .string(encode(Date(), for: f))
        default: return .null
        }
    }

    public static func encode(_ date: Date, for f: Field) -> String {
        f.type == .date ? dateFormatter.string(from: date) : datetimeFormatter.string(from: date)
    }

    public static func date(from value: JSONValue) -> Date? {
        guard let s = value.stringValue else { return nil }
        return datetimeFormatter.date(from: s) ?? dateFormatter.date(from: s)
    }

    /// 送信前の正規化。空文字/未入力は nil(キーごと落とす)。
    public static func normalize(_ v: JSONValue, for f: Field) -> JSONValue? {
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
    public static func display(_ v: JSONValue, field: Field?) -> String {
        guard let field else { return v.displayString }
        // computed が評価できなかった(ゼロ除算・参照なし)= 「—」。0 や空欄と区別する(#101)
        if field.isComputed, v.isNull { return unavailable }
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

    /// 値が出せない時の表示(Renderer の `EmptyPlaceholder.unavailableValue` と同じ)。
    public static let unavailable = "—"

    public static func currency(_ n: Double) -> String {
        let f = NumberFormatter()
        f.numberStyle = .decimal
        f.maximumFractionDigits = 0
        // DSL の currency は円前提(金額は円の整数)なので、英語 UI でも ¥ 表記のまま(桁区切りだけ Locale に従う・#44)
        // 負は「-¥500」(「¥-500」にしない・割り勘の残高など)
        let body = "¥" + (f.string(from: NSNumber(value: Swift.abs(n))) ?? "0")
        return n < 0 && n.rounded() != 0 ? "-" + body : body
    }
}
