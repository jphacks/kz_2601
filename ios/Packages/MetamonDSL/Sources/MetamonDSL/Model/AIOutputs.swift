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

    /// AI の 1 つの答え → field の値。string / text はそのまま(上限で切る)、数・日付・enum は読めた時だけ。
    public static func value(_ raw: String, field f: Field, now: Date = DSLClock.now()) -> JSONValue? {
        let s = raw.trimmingCharacters(in: .whitespacesAndNewlines)
        if s.isEmpty || blanks.contains(s.lowercased()) { return nil }
        switch f.type {
        case .string, .text:
            return .string(String(String.UnicodeScalarView(s.unicodeScalars.prefix(RecordImport.characterLimit(f)))))
        case .integer, .number:
            guard let n = RecordImport.number(s, unit: f.unit) ?? loneNumber(s) else { return nil }
            return .number(f.type == .integer ? n.rounded() : n)
        case .date, .datetime:
            let t = rangeStart(clock(s))
            guard let d = RecordImport.date(s, now: now) ?? RecordImport.date(t, now: now) else { return nil }
            return .string(FieldCoding.encode(d, for: f))
        case .enum:
            let options = f.options ?? []
            if let o = options.first(where: { $0 == s }) ?? options.first(where: { $0.lowercased() == s.lowercased() }) { return .string(o) }
            return nil
        default:
            return nil
        }
    }

    /// 書き方の注文(数・日付・enum の項目だけ)。指示の末尾に足して、読める形で答えさせる。無ければ nil。
    public static func hint(items: [String], outputs: [String: String], entity: Entity, english: Bool) -> String? {
        var parts: [String] = []
        for item in items {
            guard let id = outputs[item], let f = entity.storedFields.first(where: { $0.id == id }) else { continue }
            switch f.type {
            case .integer, .number: parts.append(english ? "\(item)=number only" : "\(item)=数字だけ")
            case .date: parts.append(english ? "\(item)=YYYY/M/D (M/D if no year)" : "\(item)=YYYY/M/D(年が無ければ M/D)")
            case .datetime: parts.append(english ? "\(item)=YYYY/M/D H:mm (M/D H:mm if no year)" : "\(item)=YYYY/M/D H:mm(年が無ければ M/D H:mm)")
            case .enum:
                let opts = (f.options ?? []).joined(separator: "/")
                parts.append(english ? "\(item)=one of \(opts)" : "\(item)=\(opts) のどれか")
            default: continue
            }
        }
        guard !parts.isEmpty else { return nil }
        let head = english ? "Format: " : "書き方: "
        let tail = english ? " (unknown: -)" : "(分からなければ -)"
        return head + String(parts.joined(separator: " / ").prefix(200)) + tail
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
