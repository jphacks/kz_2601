import Foundation

/// 式の評価値。評価できない(ゼロ除算・型不一致・参照なし)は `nil`(表示は「—」)。
public enum ExprValue: Hashable, Sendable {
    case number(Double)
    case string(String)
    case bool(Bool)
    case date(Date)

    public var number: Double? {
        switch self {
        case .number(let n): return n
        case .string(let s): return Double(s.replacingOccurrences(of: ",", with: ""))
        default: return nil
        }
    }

    public var truthy: Bool {
        switch self {
        case .number(let n): return n != 0
        case .string(let s): return !s.isEmpty && s != "false"
        case .bool(let b): return b
        case .date: return true
        }
    }

    public var text: String {
        switch self {
        case .number(let n): return JSONValue.number(n).displayString
        case .string(let s): return s
        case .bool(let b): return b ? "true" : "false"
        case .date(let d): return FieldCoding.dateFormatter.string(from: d)
        }
    }

    /// Record.data に載せる形(date は field 型に合わせて "YYYY-MM-DD" / RFC3339)。
    public func json(for field: Field? = nil) -> JSONValue {
        switch self {
        case .number(let n): return .number(n)
        case .string(let s): return .string(s)
        case .bool(let b): return .bool(b)
        case .date(let d): return .string(field?.type == .datetime ? FieldCoding.datetimeFormatter.string(from: d) : FieldCoding.dateFormatter.string(from: d))
        }
    }
}

/// `Expr` の評価器(#101)。1 画面の描画 1 回につき 1 インスタンス作り、computed / agg の結果をメモして O(n²) の再計算を避ける。
/// 行コンテキスト(`Row`)は `{"ref": ...}` の解決先。`agg` の filter.value の Expr は**外側の行**で評価する
/// (例: members の行で `{"agg":"sum","entity":"expenses","field":"amount","filter":{"field":"payer","op":"eq","value":{"ref":"id"}}}`)。
public final class ExprEvaluator {
    public struct Row {
        public var entity: Entity?
        public var record: Record
        /// 保存済みレコード(id でメモしてよい)か。フォームのドラフトは false。
        public var memoizable: Bool

        public init(entity: Entity?, record: Record, memoizable: Bool = true) {
            self.entity = entity
            self.record = record
            self.memoizable = memoizable
        }
    }

    /// 入れ子・computed の連鎖の上限(循環参照の防波堤)。
    public static let maxDepth = 48

    private let entities: [String: Entity]
    private let source: (String) -> [Record]
    private let now: Date
    private let calendar: Calendar

    private var computedMemo: [String: ExprValue?] = [:]
    private var aggMemo: [AggExpr: ExprValue?] = [:]
    private var viewMemo: [String: [Record]] = [:]
    private var inProgress: Set<String> = []
    private var depth = 0

    public init(entities: [Entity], records: @escaping (String) -> [Record], now: Date = Date(), calendar: Calendar = .current) {
        var map: [String: Entity] = [:]
        for e in entities where map[e.id] == nil { map[e.id] = e }
        self.entities = map
        self.source = records
        self.now = now
        self.calendar = calendar
    }

    public convenience init(dsl: AppDSL, records: @escaping (String) -> [Record], now: Date = Date()) {
        self.init(entities: dsl.entities, records: records, now: now)
    }

    // MARK: - Public

    public func evaluate(_ expr: Expr, row: Row? = nil) -> ExprValue? {
        guard depth < Self.maxDepth else { return nil }
        depth += 1
        defer { depth -= 1 }
        switch expr {
        case .literal(let v): return Self.value(from: v, field: nil)
        case .ref(let id):
            guard let row else { return nil }
            return fieldValue(id, row: row)
        case .agg(let a): return aggregate(a, row: row)
        case .op(let name, let args): return apply(name, args, row: row)
        case .cond(let c, let t, let e):
            return (evaluate(c, row: row)?.truthy ?? false) ? evaluate(t, row: row) : evaluate(e, row: row)
        }
    }

    /// 行のフィールド値。"id" はレコード id、computed は評価(メモ)、それ以外は保存値。
    public func fieldValue(_ fieldId: String, row: Row) -> ExprValue? {
        if fieldId == "id" { return .string(row.record.id) }
        let field = row.entity?.field(fieldId)
        guard let expr = field?.computed else { return Self.value(from: row.record[fieldId], field: field) }
        let key = "\(row.entity?.id ?? "")/\(row.record.id)/\(fieldId)"
        if row.memoizable, let hit = computedMemo[key] { return hit }
        guard !inProgress.contains(key) else { return nil } // 循環
        inProgress.insert(key)
        let v = evaluate(expr, row: row)
        inProgress.remove(key)
        if row.memoizable { computedMemo[key] = v }
        return v
    }

    /// computed を data に合成した表示用レコード(Record には保存しない)。評価できない computed はキーを落とす。
    public func viewRecords(for entityId: String) -> [Record] {
        if let hit = viewMemo[entityId] { return hit }
        let raw = source(entityId)
        guard let entity = entities[entityId], !entity.computedFields.isEmpty else { return raw }
        let out = raw.map { r -> Record in
            var r = r
            let row = Row(entity: entity, record: r)
            for f in entity.computedFields {
                r.data[f.id] = fieldValue(f.id, row: row)?.json(for: f)
            }
            return r
        }
        viewMemo[entityId] = out
        return out
    }

    /// filter.value の Expr を外側の行で評価してリテラルにする。評価できなければ null(=どれにも一致しない)。
    public func resolve(_ filters: [Filter], row: Row?) -> [Filter] {
        filters.map { f in
            guard let e = f.valueExpr else { return f }
            return Filter(field: f.field, op: f.op, value: evaluate(e, row: row)?.json() ?? .null)
        }
    }

    /// 解決済み filter の AND。
    public func apply(_ filters: [Filter], entity: Entity?, to records: [Record], row: Row? = nil) -> [Record] {
        let resolved = resolve(filters, row: row)
        guard !resolved.isEmpty else { return records }
        return records.filter { r in resolved.allSatisfy { Aggregator.matches($0, record: r, field: entity?.field($0.field)) } }
    }

    // MARK: - agg

    private func aggregate(_ a: AggExpr, row: Row?) -> ExprValue? {
        let resolved = AggExpr(op: a.op, entity: a.entity, field: a.field, filters: resolve(a.filters, row: row))
        if let hit = aggMemo[resolved] { return hit }
        let entity = entities[a.entity]
        // computed field を集計・絞り込みに使う時だけ合成済みレコードを使う(無駄な評価と自己参照を避ける)
        let usesComputed = ([a.field].compactMap { $0 } + a.filters.map(\.field)).contains { entity?.field($0)?.isComputed == true }
        let base = usesComputed ? viewRecords(for: a.entity) : source(a.entity)
        let rows = resolved.filters.isEmpty ? base : base.filter { r in
            resolved.filters.allSatisfy { Aggregator.matches($0, record: r, field: entity?.field($0.field)) }
        }
        let nums = a.field.map { f in rows.compactMap { $0[f].doubleValue ?? $0[f].stringValue.flatMap { Double($0) } } } ?? []
        let v: ExprValue?
        switch a.op {
        case "count": v = .number(Double(rows.count))
        case "sum": v = .number(nums.reduce(0, +))
        case "avg": v = nums.isEmpty ? nil : .number(nums.reduce(0, +) / Double(nums.count))
        case "min": v = nums.min().map { .number($0) }
        case "max": v = nums.max().map { .number($0) }
        default: v = nil
        }
        aggMemo[resolved] = v
        return v
    }

    // MARK: - op

    private func apply(_ name: String, _ args: [Expr], row: Row?) -> ExprValue? {
        // 遅延評価が要るもの(and / or / coalesce)を先に
        switch name {
        case "and": return .bool(args.allSatisfy { evaluate($0, row: row)?.truthy ?? false })
        case "or": return .bool(args.contains { evaluate($0, row: row)?.truthy ?? false })
        case "coalesce":
            for a in args { if let v = evaluate(a, row: row), v != .string("") { return v } }
            return nil
        default: break
        }
        let vals = args.map { evaluate($0, row: row) }
        func num(_ i: Int) -> Double? { i < vals.count ? vals[i]?.number : nil }
        func date(_ i: Int) -> Date? {
            guard i < vals.count, let v = vals[i] else { return nil }
            switch v {
            case .date(let d): return d
            case .string(let s): return FieldCoding.date(from: .string(s))
            default: return nil
            }
        }
        func allNums() -> [Double]? {
            guard !vals.isEmpty else { return nil }
            let ns = vals.compactMap { $0?.number }
            return ns.count == vals.count ? ns : nil
        }
        func finite(_ x: Double?) -> ExprValue? { x.flatMap { $0.isFinite ? .number($0) : nil } }

        switch name {
        // 算術
        case "add": return allNums().map { .number($0.reduce(0, +)) }
        case "mul": return allNums().map { .number($0.reduce(1, *)) }
        case "sub": guard let a = num(0), let b = num(1) else { return nil }; return finite(a - b)
        case "div": guard let a = num(0), let b = num(1), b != 0 else { return nil }; return finite(a / b)
        case "mod": guard let a = num(0), let b = num(1), b != 0 else { return nil }; return finite(a.truncatingRemainder(dividingBy: b))
        case "neg": return num(0).map { .number(-$0) }
        case "abs": return num(0).map { .number(Swift.abs($0)) }
        case "sqrt": guard let a = num(0), a >= 0 else { return nil }; return .number(a.squareRoot())
        case "pow": guard let a = num(0), let b = num(1) else { return nil }; return finite(Foundation.pow(a, b))
        case "round", "floor", "ceil":
            guard let a = num(0) else { return nil }
            let digits = num(1) ?? 0
            let scale = Foundation.pow(10, digits.rounded())
            let rule: FloatingPointRoundingRule = name == "floor" ? .down : name == "ceil" ? .up : .toNearestOrAwayFromZero
            return finite((a * scale).rounded(rule) / scale)
        case "min": return allNums().flatMap { $0.min() }.map { .number($0) }
        case "max": return allNums().flatMap { $0.max() }.map { .number($0) }
        case "clamp":
            guard let x = num(0), let lo = num(1), let hi = num(2), lo <= hi else { return nil }
            return .number(Swift.min(Swift.max(x, lo), hi))
        // 比較
        case "eq", "neq":
            guard vals.count >= 2 else { return nil }
            let same = Self.equal(vals[0], vals[1])
            return .bool(name == "eq" ? same : !same)
        case "gt", "gte", "lt", "lte":
            guard vals.count >= 2, let a = vals[0], let b = vals[1], let order = Self.compare(a, b) else { return nil }
            switch name {
            case "gt": return .bool(order == .orderedDescending)
            case "gte": return .bool(order != .orderedAscending)
            case "lt": return .bool(order == .orderedAscending)
            default: return .bool(order != .orderedDescending)
            }
        // 論理
        case "not": return .bool(!(vals.first??.truthy ?? false))
        // 文字列
        case "concat": return .string(vals.map { $0?.text ?? "" }.joined())
        case "len":
            guard let v = vals.first ?? nil else { return nil }
            return .number(Double(v.text.count))
        // 日付
        case "now": return .date(now)
        case "today": return .date(calendar.startOfDay(for: now))
        case "daysBetween":
            guard let a = date(0), let b = date(1) else { return nil }
            return days(from: a, to: b)
        case "daysUntil":
            guard let d = date(0) else { return nil }
            return days(from: now, to: d)
        case "addDays":
            guard let d = date(0), let n = num(1), let r = calendar.date(byAdding: .day, value: Int(n.rounded()), to: d) else { return nil }
            return .date(r)
        case "year", "month", "day", "weekday":
            guard let d = date(0) else { return nil }
            let comp: Calendar.Component = name == "year" ? .year : name == "month" ? .month : name == "day" ? .day : .weekday
            return .number(Double(calendar.component(comp, from: d)))
        // 変換
        case "toNumber":
            guard let v = vals.first ?? nil else { return nil }
            if case .bool(let b) = v { return .number(b ? 1 : 0) }
            return v.number.map { .number($0) }
        case "toString":
            guard let v = vals.first ?? nil else { return nil }
            return .string(v.text)
        default: return nil
        }
    }

    private func days(from a: Date, to b: Date) -> ExprValue? {
        calendar.dateComponents([.day], from: calendar.startOfDay(for: a), to: calendar.startOfDay(for: b)).day.map { .number(Double($0)) }
    }

    // MARK: - 値の変換・比較

    /// 保存値 → 評価値。date/datetime field の文字列は Date にする。null は nil。
    public static func value(from v: JSONValue, field: Field?) -> ExprValue? {
        switch v {
        case .number(let n): return .number(n)
        case .bool(let b): return .bool(b)
        case .string(let s):
            if field?.type.isDate == true, let d = FieldCoding.date(from: v) { return .date(d) }
            return .string(s)
        case .null, .array, .object: return nil
        }
    }

    static func equal(_ a: ExprValue?, _ b: ExprValue?) -> Bool {
        switch (a, b) {
        case (nil, nil): return true
        case (nil, _), (_, nil): return false
        case let (.number(x)?, .number(y)?): return x == y
        case let (.date(x)?, .date(y)?): return x == y
        default:
            if let x = a?.number, let y = b?.number { return x == y }
            return a?.text == b?.text
        }
    }

    static func compare(_ a: ExprValue, _ b: ExprValue) -> ComparisonResult? {
        switch (a, b) {
        case let (.date(x), .date(y)): return x == y ? .orderedSame : (x < y ? .orderedAscending : .orderedDescending)
        case (.string(let x), .string(let y)) where Double(x) == nil || Double(y) == nil: return x.compare(y)
        default:
            guard let x = a.number, let y = b.number else { return nil }
            return x == y ? .orderedSame : (x < y ? .orderedAscending : .orderedDescending)
        }
    }
}
