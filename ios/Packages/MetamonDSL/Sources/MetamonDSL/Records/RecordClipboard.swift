import Foundation

/// 記録を項目つきでコピーし、別の生成アプリのフォームに貼る(#1871 の iOS 分)。読み書きはホスト、ここは純粋ロジックだけ。
///
/// - コピー: 載せる項目は保存フィールドのうち、hidden・暗号化・端末のファイル(photo / video / audio)・
///   アプリが違うと意味を持たない参照(ref / refs / member / members)以外。null・型の合わない値も載せない
/// - 貼り付け: 1. 同じ entity id で field の id と型が合う → 2. 名前が同じで型が変換できる。合わないものは捨てる
public enum RecordClipboard {
    public static let version = 1
    /// クリップボードに載せる独自の型(Info.plist の UTExportedTypeDeclarations と一致させる)。
    public static let typeIdentifier = "app.tailorit.record"
    /// クリップボードに置いておく時間。過ぎたら OS が消す。
    public static let expiry: TimeInterval = 10 * 60
    /// 読む・書くデータの上限。超えたら読まない。
    public static let maxBytes = 256 * 1024
    /// 載せる項目の上限。
    public static let maxFields = NearbyLimits.maxFields

    public struct FieldValue: Codable, Equatable, Sendable {
        public var id: String
        public var name: String
        public var type: String
        public var value: JSONValue
        public init(id: String, name: String, type: String, value: JSONValue) {
            self.id = id; self.name = name; self.type = type; self.value = value
        }
    }

    public struct Payload: Codable, Equatable, Sendable {
        public var v: Int
        public var appId: String
        public var appName: String
        public var entityId: String
        public var entityName: String
        public var fields: [FieldValue]
        public init(v: Int = RecordClipboard.version, appId: String, appName: String, entityId: String, entityName: String,
                    fields: [FieldValue]) {
            self.v = v; self.appId = appId; self.appName = appName
            self.entityId = entityId; self.entityName = entityName; self.fields = fields
        }
    }

    /// 貼り付けの結果。`values` をフォームに入れ、`matched` / `unmatched` を利用者に見せる。
    public struct PasteResult: Equatable, Sendable {
        /// 貼る先の field id → 値。
        public var values: [String: JSONValue]
        /// 入った項目(貼る先の field の名前・フォームの順)。
        public var matched: [String]
        /// 合わなかった項目(コピー元の field の名前・コピー元の順)。
        public var unmatched: [String]
        public init(values: [String: JSONValue], matched: [String], unmatched: [String]) {
            self.values = values; self.matched = matched; self.unmatched = unmatched
        }
    }

    /// 載せない型: 端末のファイルの参照と、アプリが違うと id が意味を持たない参照。
    static let excludedTypes: Set<FieldType> = [.photo, .video, .audio, .ref, .refs, .member, .members]

    /// コピー・貼り付けの対象になる field(保存フィールドのうち hidden・暗号化・`excludedTypes` を除く)。
    public static func copyableFields(of entity: Entity) -> [Field] {
        entity.storedFields.filter { !$0.hidden && !$0.encrypted && !excludedTypes.contains($0.type) }
    }

    // MARK: - コピー

    /// 1 件の記録から載せるものを作る。載せる項目が 1 つも無ければ nil(メニューを出さない)。
    public static func make(appId: String, appName: String, entity: Entity, data: [String: JSONValue]) -> Payload? {
        let fields = copyableFields(of: entity).prefix(maxFields).compactMap { f -> FieldValue? in
            guard let v = data[f.id], !v.isNull, NearbyShare.accepts(v, f.type) else { return nil }
            return FieldValue(id: f.id, name: f.name, type: f.type.rawValue, value: v)
        }
        guard !fields.isEmpty else { return nil }
        return Payload(v: version, appId: appId, appName: appName, entityId: entity.id, entityName: entity.name, fields: fields)
    }

    /// 文字として貼る先(メモ・メール)向けの「名前: 値」の行。`display` は値を画面と同じ書き方にする(無ければ素の値)。
    public static func plainText(_ payload: Payload, display: (FieldValue) -> String = { rawText($0.value) }) -> String {
        payload.fields.compactMap { f -> String? in
            let value = display(f).trimmingCharacters(in: .whitespacesAndNewlines)
            guard !value.isEmpty else { return nil }
            return f.name.isEmpty ? value : "\(f.name): \(value)"
        }.joined(separator: "\n")
    }

    public static func rawText(_ v: JSONValue) -> String {
        switch v {
        case .string(let s): s
        case .number(let n): numberText(n)
        case .bool(let b): b ? "true" : "false"
        case .array(let a): a.map(rawText).joined(separator: ", ")
        case .null, .object: ""
        }
    }

    public static func numberText(_ n: Double) -> String {
        n.rounded() == n && abs(n) < 1e15 ? String(Int64(n)) : String(n)
    }

    // MARK: - 符号化

    public static func encode(_ payload: Payload) -> Data? {
        guard let data = try? NearbyShare.encode(payload), data.count <= maxBytes else { return nil }
        return data
    }

    /// クリップボードの中身を読む。大きすぎる・新しすぎる・id が壊れたものは nil。名前は上限に切り詰める。
    public static func decode(_ data: Data) -> Payload? {
        guard data.count <= maxBytes, var p = try? NearbyShare.decode(Payload.self, from: data), p.v <= version,
              NearbyLimits.isValidId(p.entityId), p.appId.count <= NearbyLimits.maxIdLength else { return nil }
        p.appName = NearbyLimits.clip(p.appName, NearbyLimits.maxLabelLength)
        p.entityName = NearbyLimits.clip(p.entityName, NearbyLimits.maxLabelLength)
        p.fields = p.fields.prefix(maxFields).compactMap { f in
            guard NearbyLimits.isValidId(f.id), NearbyLimits.isValidId(f.type) else { return nil }
            var f = f
            f.name = NearbyLimits.clip(f.name, NearbyLimits.maxLabelLength)
            return f
        }
        return p
    }

    // MARK: - 貼り付け

    /// 貼る先の entity に入れる値を決める。同じ entity なら id で、それ以外は名前(前後の空白・大文字小文字を無視)で合わせる。
    /// 貼る先の 1 つの field には 1 回だけ入れる。
    public static func paste(_ payload: Payload, into entity: Entity) -> PasteResult {
        let targets = copyableFields(of: entity)
        let sameEntity = payload.entityId == entity.id
        var used: Set<String> = []
        var values: [String: JSONValue] = [:]
        var unmatched: [String] = []
        for src in payload.fields {
            let byId = sameEntity ? targets.first { $0.id == src.id && !used.contains($0.id) } : nil
            let candidates = (byId.map { [$0] } ?? []) + targets.filter { !used.contains($0.id) && nameKey($0.name) == nameKey(src.name) && $0.id != byId?.id }
            var placed = false
            for target in candidates {
                if let v = coerce(src.value, from: src.type, to: target) {
                    values[target.id] = v
                    used.insert(target.id)
                    placed = true
                    break
                }
            }
            if !placed { unmatched.append(src.name.isEmpty ? src.id : src.name) }
        }
        let matched = targets.filter { values[$0.id] != nil }.map(\.name)
        return PasteResult(values: values, matched: matched, unmatched: unmatched)
    }

    static func nameKey(_ s: String) -> String {
        s.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
    }

    /// コピー元の値を貼る先の field の型に変える。変えられなければ nil。
    /// 文字(string ↔ text・enum → 文字)、数(integer → number / currency、整数の number → integer、数字だけの文字 → 数)、
    /// 選択肢(選べる値だけ)、日時 → 日付、同じ型どうし。
    public static func coerce(_ value: JSONValue, from sourceType: String, to target: Field) -> JSONValue? {
        guard let src = FieldType(rawValue: sourceType), !excludedTypes.contains(src), NearbyShare.accepts(value, src) else { return nil }
        let t = target.type
        switch (src, value) {
        case (_, .string(let s)) where src == .string || src == .text || src == .enum:
            switch t {
            case .string, .text: return value
            case .enum: return (target.options ?? []).contains(s) ? value : JSONValue?.none
            case .integer, .number, .currency:
                guard let n = Double(s.trimmingCharacters(in: .whitespaces)), n.isFinite else { return nil }
                return numeric(n, to: t)
            default: return nil
            }
        case (_, .number(let n)) where src.isNumeric:
            switch t {
            case .integer, .number, .currency: return numeric(n, to: t)
            case .string, .text: return .string(numberText(n))
            default: return nil
            }
        case (.boolean, .bool): return t == .boolean ? value : JSONValue?.none
        case (.date, .string): return t == .date ? value : JSONValue?.none
        case (.datetime, .string(let s)):
            if t == .datetime { return value }
            guard t == .date, let d = FieldCoding.datetimeFormatter.date(from: s) else { return nil }
            return .string(FieldCoding.dateFormatter.string(from: d))
        case (.location, .string): return t == .location ? value : JSONValue?.none
        default: return nil
        }
    }

    private static func numeric(_ n: Double, to t: FieldType) -> JSONValue? {
        if t == .integer, n.rounded() != n { return nil }
        return .number(n)
    }
}
