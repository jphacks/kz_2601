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
        // 選択肢から消えた既定値は使わない(#194)
        if let d = f.defaultValue, !(f.type == .enum && !(f.options ?? []).contains(d.stringValue ?? "")) {
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
            // JSONValue は ExpressibleByNilLiteral のため三項の `nil` は `.null` になる(#121)。Optional.none を明示する。
            if v.isNull { return Optional<JSONValue>.none }
            return v
        }
    }

    /// display が出す固定の語(#150)。DSL パッケージは文言カタログを持たないので、Renderer が `L()` で引いた語を渡す。既定は日本語。
    public struct Words: Sendable, Equatable {
        public var yes: String
        public var no: String
        public var photo: String
        public var video: String
        public init(yes: String, no: String, photo: String, video: String = "動画") {
            self.yes = yes
            self.no = no
            self.photo = photo
            self.video = video
        }
        public static let ja = Words(yes: "はい", no: "いいえ", photo: "写真", video: "動画")
    }

    /// 一覧・集計の表示用整形。`locale` / `now` は日付の書式(言語・今年なら年を省く)にだけ使う。`words` は boolean / photo の語。
    public static func display(_ v: JSONValue, field: Field?, locale: Locale = .current, now: Date = Date(), words: Words = .ja) -> String {
        guard let field else { return v.displayString }
        // computed が評価できなかった(ゼロ除算・参照なし)= 「—」。0 や空欄と区別する(#101)
        if field.isComputed, v.isNull { return unavailable }
        switch field.type {
        case .currency:
            // unit が「円」等の通貨そのものなら付けない(¥ と二重になる)。「円/時」なら「¥1,200/時」(#150)
            let body = currency(v.doubleValue ?? 0)
            guard let suffix = currencySuffix(unit: field.unit) else { return body }
            return body + (suffix.hasPrefix("/") ? "" : " ") + suffix
        case .number, .integer:
            guard let n = v.doubleValue else { return "" }
            let s = n == n.rounded() ? String(Int(n)) : String(format: "%.1f", n)
            return field.unit.map { s + " " + $0 } ?? s
        case .date, .datetime:
            guard let d = date(from: v) else { return v.displayString }
            return displayDate(d, withTime: field.type == .datetime, locale: locale, now: now)
        case .boolean: return v.truthy ? words.yes : words.no
        case .photo: return v.isNull ? "" : words.photo
        case .video: return v.isNull ? "" : words.video
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

    /// 通貨そのものを表す unit(大小文字は無視)。currency は ¥ を前に出すので、これらは unit として重ねない(#150)。
    /// "$" / USD も含める: 通貨は常に ¥ 表示(案 B)なので、生成が "$" を書いても「¥15 $」にしない。
    static let currencyUnitTokens = ["円", "¥", "￥", "jpy", "yen", "$", "usd", "dollar", "dollars"]

    /// currency field の unit のうち、¥ の後ろに付ける残り(#150)。
    /// 「円」「JPY」→ nil(¥ だけ)/「円/時」→「/時」/「/h」「per hour」→ そのまま / 空 → nil。
    public static func currencySuffix(unit: String?) -> String? {
        guard let raw = unit?.trimmingCharacters(in: .whitespaces), !raw.isEmpty else { return nil }
        let lower = raw.lowercased()
        if currencyUnitTokens.contains(lower) { return nil }
        // 長い語から当てる("dollars" を "dollar" + "s" と切らない)
        for token in currencyUnitTokens.sorted(by: { $0.count > $1.count }) where lower.hasPrefix(token) {
            let rest = raw.dropFirst(token.count).trimmingCharacters(in: .whitespaces)
            return rest.isEmpty ? nil : rest
        }
        return raw
    }

    /// 日付の表示(#140)。ja「10/24(金)」「10/24(金) 14:30」、en「Fri, Oct 24」「Fri, Oct 24, 2:30 PM」。今年でなければ年を付ける。
    public static func displayDate(_ d: Date, withTime: Bool, locale: Locale, now: Date = Date()) -> String {
        let cal = Calendar(identifier: .gregorian)
        let sameYear = cal.component(.year, from: d) == cal.component(.year, from: now)
        let f = DateFormatter()
        f.locale = locale
        if locale.language.languageCode?.identifier == "ja" {
            f.dateFormat = (sameYear ? "M/d(EEE)" : "yyyy/M/d(EEE)") + (withTime ? " H:mm" : "")
        } else {
            f.setLocalizedDateFormatFromTemplate((sameYear ? "EEEMMMd" : "EEEMMMdy") + (withTime ? "jmm" : ""))
        }
        return f.string(from: d)
    }
}
