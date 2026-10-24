import Foundation

/// Siri / ショートカットの「記録を追加」(#80)用: 一言(「ランチ 800円」)から entity の 1 レコードを組み立てる。
/// DSL を変えずに、どの生成アプリでも動くよう Field の型だけで当てはめる:
/// - タイトル系(titleField → 最初の string/text): 数値を除いた文言。enum ならオプションに一致する値
/// - 数値 1 つ(最初の required な数値 → 最初の数値): 文中の数値(またはパラメータ)
/// - それ以外: フォームと同じ既定値(`FieldCoding.defaultValue`。date の now・enum の先頭など)
public enum QuickRecord {
    public struct Parsed: Equatable, Sendable {
        public var title: String?
        public var number: Double?
    }

    public struct Draft: Equatable, Sendable {
        /// appconfig に送る `data`(`FormDraft.payload` と同じ正規化済み)
        public var data: [String: JSONValue]
        /// 埋められなかった required 項目の表示名
        public var missing: [String]
        /// 応答用の短い要約(例: 「ランチ ¥800」)
        public var summary: String
    }

    // MARK: - Parse

    /// 「ランチ 800円」→ (ランチ, 800)。全角数字・桁区切り・小数・¥/円/kg/回 などの単位を許す。数値が無ければ title だけ。
    public static func parse(_ raw: String) -> Parsed {
        let text = halfwidthDigits(raw).trimmingCharacters(in: .whitespacesAndNewlines)
        guard !text.isEmpty else { return Parsed(title: nil, number: nil) }
        // 末尾側の数値を優先(「3月の食費 1200円」→ 1200)
        let pattern = #"[¥$]?\s*(\d[\d,]*(?:\.\d+)?)([^\s\d、。,.]{0,3})"#
        guard let regex = try? NSRegularExpression(pattern: pattern),
              let match = regex.matches(in: text, range: NSRange(text.startIndex..., in: text)).last,
              let numRange = Range(match.range(at: 1), in: text),
              let whole = Range(match.range, in: text),
              let number = Double(text[numRange].replacingOccurrences(of: ",", with: ""))
        else { return Parsed(title: cleanTitle(text), number: nil) }
        // 単位っぽい後続(円/kg/回…)は落とすが、「800円のランチ」の「のラ」のような助詞+語は残す
        var unitEnd = whole.upperBound
        if let unitRange = Range(match.range(at: 2), in: text) {
            let unit = String(text[unitRange])
            let kept = unit.prefix { !particles.contains($0) }
            unitEnd = text.index(unitRange.lowerBound, offsetBy: kept.count)
        }
        let rest = String(text[..<whole.lowerBound]) + " " + String(text[unitEnd...])
        return Parsed(title: cleanTitle(rest), number: number)
    }

    /// 全角の数字・記号だけ半角に(カタカナは変えない)。
    static func halfwidthDigits(_ s: String) -> String {
        String(String.UnicodeScalarView(s.unicodeScalars.map { u in
            switch u.value {
            case 0xFF10...0xFF19, 0xFF0C, 0xFF0E: Unicode.Scalar(u.value - 0xFEE0) ?? u // ０-９ ， ．
            case 0xFFE5: "¥"
            case 0x3000: " "
            default: u
            }
        }))
    }

    private static let particles: Set<Character> = ["を", "に", "で", "の", "は", "が", "と"]

    private static func cleanTitle(_ s: String) -> String? {
        var t = s.trimmingCharacters(in: .whitespacesAndNewlines.union(.init(charactersIn: "、。,.")))
        while let first = t.first, particles.contains(first) { t.removeFirst(); t = t.trimmingCharacters(in: .whitespaces) }
        while let last = t.last, particles.contains(last) { t.removeLast(); t = t.trimmingCharacters(in: .whitespaces) }
        t = t.split(whereSeparator: \.isWhitespace).joined(separator: " ")
        return t.isEmpty ? nil : t
    }

    // MARK: - Field 選択

    /// タイトルに当てる項目: titleField(string/text/enum)→ 最初の string/text。
    public static func titleField(of entity: Entity) -> Field? {
        if let id = entity.titleField, let f = entity.field(id), [.string, .text, .enum].contains(f.type) { return f }
        return entity.fields.first { $0.type == .string || $0.type == .text }
    }

    /// 数値に当てる項目: 最初の required な数値 → 最初の数値。
    public static func numberField(of entity: Entity) -> Field? {
        entity.fields.first { $0.type.isNumeric && ($0.required ?? false) } ?? entity.fields.first { $0.type.isNumeric }
    }

    /// 「記録を追加」の対象に出す entity。フォーム画面(`screen.entity`)を持つものを先に、項目の無いものは除く。
    public static func recordableEntities(in dsl: AppDSL) -> [Entity] {
        let withForm = Set(dsl.screens.compactMap(\.entity))
        let usable = dsl.entities.filter { !$0.fields.isEmpty }
        return usable.filter { withForm.contains($0.id) } + usable.filter { !withForm.contains($0.id) }
    }

    // MARK: - Build

    /// `text`(一言)と `number`(明示パラメータ。nil なら文中の数値)から 1 レコード分の `data` を作る。
    public static func build(entity: Entity, text: String?, number explicit: Double? = nil) -> Draft {
        let parsed = parse(text ?? "")
        let number = explicit ?? parsed.number
        var values: [String: JSONValue] = [:]
        for f in entity.fields { values[f.id] = FieldCoding.defaultValue(for: f) }

        let titleF = titleField(of: entity)
        let numberF = numberField(of: entity)
        let rawText = text ?? ""
        // 文中にオプション名がある enum は全部それに合わせる(「交通費 300円」→ category=交通費)
        for f in entity.fields where f.type == .enum {
            if let hit = matchOption(in: rawText, options: f.options ?? []) { values[f.id] = .string(hit) }
        }
        if let titleF, let title = parsed.title {
            if titleF.type == .enum {
                if let hit = matchOption(in: title, options: titleF.options ?? []) { values[titleF.id] = .string(hit) }
                else if titleF.options?.isEmpty ?? true { values[titleF.id] = .string(title) }
            } else {
                values[titleF.id] = .string(title)
            }
        }
        if let numberF, let number { values[numberF.id] = .number(number) }

        var data: [String: JSONValue] = [:]
        for f in entity.fields {
            // normalize は未入力を `.null` で返すことがある(ExpressibleByNilLiteral)ので明示的に落とす
            if let v = FieldCoding.normalize(values[f.id] ?? .null, for: f), !v.isNull { data[f.id] = v }
        }
        let missing = entity.fields.filter { ($0.required ?? false) && data[$0.id] == nil }.map(\.name)
        let parts = [titleF, numberF].compactMap { $0 }.compactMap { f in data[f.id].map { FieldCoding.display($0, field: f) } }
        return Draft(data: data, missing: missing, summary: parts.filter { !$0.isEmpty }.joined(separator: " "))
    }

    /// 最長一致のオプション(「ベンチプレス」と「ベンチ」なら長い方)。
    static func matchOption(in text: String, options: [String]) -> String? {
        let t = text.lowercased()
        return options.filter { !$0.isEmpty && t.contains($0.lowercased()) }.max { $0.count < $1.count }
    }
}
