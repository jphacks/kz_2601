import Foundation

/// `aiButton.outputs`: generate の fields の項目名 → フォームの field id。検証は backend `dsl/ai_outputs.go`。
/// 結果を field の型に直してフォームのドラフトに入れる(保存は利用者)。読めない値は入れない(空のまま・入力済みは消さない)。
public enum AIOutputs {
    /// 1 項目の入れ先と、入れる値(読めなければ nil)。
    public struct Fill: Equatable, Sendable {
        public var item: String
        public var field: String
        public var fieldName: String
        public var value: JSONValue?
    }

    static let types: Set<FieldType> = [.string, .text, .integer, .number, .date, .datetime, .enum]
    /// 「分からない」の答え。どの型でも空のままにする。
    static let blanks: Set<String> = ["-", "－", "ー", "—", "―", "?", "？", "不明", "なし", "無し", "未定", "不詳", "n/a", "na", "none", "unknown", "tbd"]

    /// 結果の fields を outputs の順(items の順)で入れ先に対応させる。outputs に無い項目・入れられない field は飛ばす。
    public static func fills(_ fields: [AITextField], outputs: [String: String], entity: Entity, now: Date = DSLClock.now()) -> [Fill] {
        var map: [String: String] = [:]
        for (k, v) in outputs { map[k.trimmingCharacters(in: .whitespaces)] = v }
        return fields.compactMap { f in
            let name = f.name.trimmingCharacters(in: .whitespaces)
            guard let id = map[name], let field = entity.storedFields.first(where: { $0.id == id }), types.contains(field.type) else { return nil }
            return Fill(item: name, field: id, fieldName: field.name, value: value(f.value, field: field, now: now))
        }
    }

    /// 入れた結果。`applied` は入れた(同じ値だった)項目、`kept` は入力済みで値が違うため入れなかった項目。
    public struct Applied: Equatable, Sendable {
        public var applied: [Fill] = []
        public var kept: [Fill] = []

        public init(applied: [Fill] = [], kept: [Fill] = []) {
            self.applied = applied
            self.kept = kept
        }
    }

    /// 読めた値をドラフトに入れる。上書きしない: 空の field と、`replaceable`(新規で開いたままの初期値・前回 AI が入れた値のまま)だけ。
    public static func apply(_ fills: [Fill], to values: inout [String: JSONValue],
                             replaceable: (_ field: String, _ current: JSONValue) -> Bool) -> Applied {
        var out = Applied()
        for f in fills {
            guard let v = f.value else { continue }
            let cur = values[f.field] ?? .null
            if cur == v || isBlank(cur) || replaceable(f.field, cur) {
                values[f.field] = v
                out.applied.append(f)
            } else {
                out.kept.append(f)
            }
        }
        return out
    }

    /// 入力が無い値(null・空白だけの文字)。
    static func isBlank(_ v: JSONValue) -> Bool {
        switch v {
        case .null: true
        case .string(let s): s.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
        default: false
        }
    }

    /// AI の 1 つの答え → field の値。string / text はそのまま(上限で切る)、数・日付・enum は読めた時だけ。
    public static func value(_ raw: String, field f: Field, now: Date = DSLClock.now()) -> JSONValue? {
        let s = raw.trimmingCharacters(in: .whitespacesAndNewlines)
        if s.isEmpty || blanks.contains(s.lowercased()) { return nil }
        switch f.type {
        case .string, .text:
            return .string(String(String.UnicodeScalarView(s.unicodeScalars.prefix(RecordImport.characterLimit(f)))))
        case .integer, .number:
            let n: Double?
            if let plain = RecordImport.number(s, unit: f.unit) {
                n = plain
            } else if let k = kanjiNumber(s) {
                n = k // 万・千・億がある: 読めなければ nil(「3万円」を 3 にしない)
            } else {
                n = loneNumber(s)
            }
            guard let n else { return nil }
            return .number(f.type == .integer ? n.rounded() : n)
        case .date, .datetime:
            let t = rangeStart(clock(s))
            // date は書かれたオフセットのままの日付(端末の TZ に直すと日がずれる)
            if f.type == .date, let day = isoDay(t) { return .string(day) }
            guard var d = RecordImport.date(s, now: now) ?? RecordImport.date(t, now: now) else { return nil }
            if !hasYear(t), let next = nextYearIfPast(d, now: now) { d = next }
            return .string(FieldCoding.encode(d, for: f))
        case .enum:
            let options = f.options ?? []
            if let o = options.first(where: { $0 == s }) ?? options.first(where: { $0.lowercased() == s.lowercased() }) { return .string(o) }
            return nil
        default:
            return nil
        }
    }

    /// 書き方の注文(数・日付・enum の項目だけ)。`POST /v1/text` の formats(書き方の文はサーバーが足す)。無ければ nil。
    public static func formats(items: [String], outputs: [String: String], entity: Entity) -> [AITextFormat]? {
        var out: [AITextFormat] = []
        for item in items {
            guard let id = outputs[item], let f = entity.storedFields.first(where: { $0.id == id }) else { continue }
            switch f.type {
            case .integer, .number: out.append(AITextFormat(item: item, kind: "number"))
            case .date: out.append(AITextFormat(item: item, kind: "date"))
            case .datetime: out.append(AITextFormat(item: item, kind: "datetime"))
            case .enum:
                // backend の上限(20 個・各 40 字)に収める
                let opts = (f.options ?? []).map { $0.trimmingCharacters(in: .whitespaces) }
                    .filter { !$0.isEmpty && $0.count <= 40 }.prefix(20)
                if !opts.isEmpty { out.append(AITextFormat(item: item, kind: "enum", options: Array(opts))) }
            default: continue
            }
        }
        return out.isEmpty ? nil : out
    }

    /// formats の書き方の文(端末内 AI 用・backend `textFormatHint` と同じ文)。無ければ nil。
    public static func hint(_ formats: [AITextFormat]?, english: Bool) -> String? {
        guard let formats, !formats.isEmpty else { return nil }
        let parts = formats.map { f -> String in
            let s: String
            switch f.kind {
            case "number": s = english ? "number only" : "数字だけ"
            case "date": s = english ? "YYYY/M/D (M/D if no year)" : "YYYY/M/D(年が無ければ M/D)"
            case "datetime": s = english ? "YYYY/M/D H:mm (M/D H:mm if no year)" : "YYYY/M/D H:mm(年が無ければ M/D H:mm)"
            default: s = (f.options ?? []).joined(separator: "/") + (english ? " (one of these)" : " のどれか")
            }
            return "\(f.item)=\(s)"
        }
        return (english ? "Format: " : "書き方: ") + parts.joined(separator: " / ") + (english ? " (unknown: -)" : "(分からなければ -)")
    }

    /// 「85 点」「約 3,000 円」: 数がちょうど 1 つの時だけその数。「85/100」のように 2 つあれば読まない。
    static func loneNumber(_ s: String) -> Double? {
        let t = s.applyingTransform(.fullwidthToHalfwidth, reverse: false) ?? s
        let re = try! NSRegularExpression(pattern: #"-?\d[\d,]*(?:\.\d+)?"#)
        let ns = t as NSString
        let hits = re.matches(in: t, range: NSRange(location: 0, length: ns.length))
        guard hits.count == 1 else { return nil }
        let n = Double(ns.substring(with: hits[0].range).replacingOccurrences(of: ",", with: ""))
        return n.flatMap { $0.isFinite ? $0 : nil }
    }

    private static let kanjiPattern = try! NSRegularExpression(pattern:
        #"^(?:約|およそ|だいたい)?(-)?(?:(\d+(?:\.\d+)?)億)?(?:(\d+(?:\.\d+)?)(千)?万)?(?:(\d+(?:\.\d+)?)千)?(\d+(?:\.\d+)?)?(?:円|人|点|件|個|回|本|枚|冊|歩|名|ドル|円くらい|円ほど|円程度|人程度|程度|くらい|ほど)?$"#)

    /// 「3万円」「約1.5万人」「1億2000万」「3千万」「5千」: 万・千・億の数。
    /// - 万・千・億が無ければ nil(ほかの読み方へ)。あって読めない(「3万〜5万」「3万円以上」)なら .some(nil) = 空のまま
    static func kanjiNumber(_ s: String) -> Double?? {
        var t = s.applyingTransform(.fullwidthToHalfwidth, reverse: false) ?? s
        guard t.contains(where: { "万千億".contains($0) }) else { return nil }
        for token in [",", " ", "¥"] { t = t.replacingOccurrences(of: token, with: "") }
        let ns = t as NSString
        guard let m = kanjiPattern.firstMatch(in: t, range: NSRange(location: 0, length: ns.length)) else { return .some(nil) }
        func num(_ i: Int) -> Double? {
            let r = m.range(at: i)
            return r.location == NSNotFound ? nil : Double(ns.substring(with: r))
        }
        let oku = num(2), man = num(3), sen = num(5), rest = num(6)
        guard oku != nil || man != nil || sen != nil else { return .some(nil) }
        let manScale: Double = m.range(at: 4).location == NSNotFound ? 1e4 : 1e7
        var n = (oku ?? 0) * 1e8 + (man ?? 0) * manScale + (sen ?? 0) * 1e3 + (rest ?? 0)
        if m.range(at: 1).location != NSNotFound { n = -n }
        return n.isFinite ? .some(n) : .some(nil)
    }

    private static let isoDayPattern = try! NSRegularExpression(
        pattern: #"^([0-9]{4}-[0-9]{2}-[0-9]{2})T[0-9]{2}:[0-9]{2}(?::[0-9]{2}(?:\.[0-9]+)?)?(?:Z|[+-][0-9]{2}:?[0-9]{2})?$"#)

    /// 「2026-11-20T23:30:00-05:00」の日付の部分(オフセットのまま)。正しい日でなければ nil。
    static func isoDay(_ s: String) -> String? {
        let ns = s as NSString
        guard let m = isoDayPattern.firstMatch(in: s, range: NSRange(location: 0, length: ns.length)) else { return nil }
        let day = ns.substring(with: m.range(at: 1))
        return FieldCoding.strictDay(day) == nil ? nil : day
    }

    /// 年が書いてあるか(4 桁の数)。
    static func hasYear(_ s: String) -> Bool {
        s.range(of: #"\d{4}"#, options: .regularExpression) != nil
    }

    /// 年なしの日付が今日より 30 日以上前なら来年(12 月に「1/10」→ 翌年 1 月)。そうでなければ nil。
    static func nextYearIfPast(_ d: Date, now: Date) -> Date? {
        guard d < now.addingTimeInterval(-30 * 86_400) else { return nil }
        var cal = Calendar(identifier: .gregorian)
        cal.timeZone = FieldCoding.dateFormatter.timeZone
        return cal.date(byAdding: .year, value: 1, to: d)
    }

    /// 「18時30分」「18時」→「18:30」「18:00」(全角数字も)。
    static func clock(_ s: String) -> String {
        var t = s.applyingTransform(.fullwidthToHalfwidth, reverse: false) ?? s
        t = t.replacingOccurrences(of: #"(\d{1,2})時(\d{1,2})分"#, with: "$1:$2", options: .regularExpression)
        t = t.replacingOccurrences(of: #"(\d{1,2})時"#, with: "$1:00", options: .regularExpression)
        return t.replacingOccurrences(of: #"日((?:\([^)]*\))?)(\d)"#, with: "日$1 $2", options: .regularExpression)
    }

    /// 「18:00〜20:00」「11/20 - 11/22」は始まりだけ。
    static func rangeStart(_ s: String) -> String {
        for sep in ["〜", "～", "~", " - ", "–"] {
            if let r = s.range(of: sep) { return String(s[..<r.lowerBound]).trimmingCharacters(in: .whitespaces) }
        }
        return s
    }
}
