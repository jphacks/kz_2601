import Foundation

/// 計算式(Issue #101・backend #100 と共通契約)。JSON 表現:
/// ```
/// Expr := number | string | boolean
///       | {"ref": "<fieldId>"}                       行コンテキストのフィールド値("id" でレコード id)
///       | {"agg": "count|sum|avg|min|max", "entity": "...", "field"?: "...", "filter"?: Filter | [Filter]}
///       | {"op": "<name>", "args": [Expr, ...]}
///       | {"if": Expr, "then": Expr, "else": Expr}
/// ```
/// 正典は `docs/dsl/expressions.md` / `dsl.schema.json` の `Expr`(ExprRef / ExprAgg / ExprOp / ExprIf)。
/// 契約上リテラルに null は無いが、デコードは寛容に受けて `.literal(.null)` にする(then/else 省略時の既定値にも使う)。
/// 評価は `ExprEvaluator`。形の壊れた式はデコードで throw する(呼び出し側が try? で握る)。
public indirect enum Expr: Hashable, Sendable {
    case literal(JSONValue)
    case ref(String)
    case agg(AggExpr)
    case op(String, [Expr])
    case cond(Expr, then: Expr, else: Expr)

    /// リテラル(number/string/bool/null)ならその値。Filter.value の後方互換に使う。
    public var literalValue: JSONValue? { if case .literal(let v) = self { return v }; return nil }
}

public struct AggExpr: Hashable, Sendable {
    public var op: String          // count|sum|avg|min|max
    public var entity: String
    public var field: String?
    /// AND で全部満たすレコードだけ集計する。
    public var filters: [Filter]

    public init(op: String, entity: String, field: String? = nil, filters: [Filter] = []) {
        self.op = op
        self.entity = entity
        self.field = field
        self.filters = filters
    }
}

extension Expr: Codable {
    private enum Keys: String, CodingKey { case ref, agg, entity, field, filter, op, args, `if`, then, `else` }

    public init(from decoder: Decoder) throws {
        if let single = try? decoder.singleValueContainer() {
            if single.decodeNil() { self = .literal(.null); return }
            if let b = try? single.decode(Bool.self) { self = .literal(.bool(b)); return }
            if let n = try? single.decode(Double.self) { self = .literal(.number(n)); return }
            if let s = try? single.decode(String.self) { self = .literal(.string(s)); return }
        }
        let c = try decoder.container(keyedBy: Keys.self)
        if let r = try c.decodeIfPresent(String.self, forKey: .ref) {
            self = .ref(r)
        } else if let a = try c.decodeIfPresent(String.self, forKey: .agg) {
            self = .agg(AggExpr(op: a, entity: try c.decode(String.self, forKey: .entity),
                                field: try c.decodeIfPresent(String.self, forKey: .field),
                                filters: Filter.decodeOneOrMany(c, forKey: .filter)))
        } else if let o = try c.decodeIfPresent(String.self, forKey: .op) {
            self = .op(o, try c.decodeIfPresent([Expr].self, forKey: .args) ?? [])
        } else if c.contains(.if) {
            self = .cond(try c.decode(Expr.self, forKey: .if),
                         then: try c.decodeIfPresent(Expr.self, forKey: .then) ?? .literal(.null),
                         else: try c.decodeIfPresent(Expr.self, forKey: .else) ?? .literal(.null))
        } else {
            throw DecodingError.dataCorrupted(.init(codingPath: decoder.codingPath, debugDescription: "Expr: ref/agg/op/if のいずれも無い"))
        }
    }

    public func encode(to encoder: Encoder) throws {
        switch self {
        case .literal(let v):
            try v.encode(to: encoder)
        case .ref(let r):
            var c = encoder.container(keyedBy: Keys.self)
            try c.encode(r, forKey: .ref)
        case .agg(let a):
            var c = encoder.container(keyedBy: Keys.self)
            try c.encode(a.op, forKey: .agg)
            try c.encode(a.entity, forKey: .entity)
            try c.encodeIfPresent(a.field, forKey: .field)
            if a.filters.count == 1 { try c.encode(a.filters[0], forKey: .filter) }
            else if !a.filters.isEmpty { try c.encode(a.filters, forKey: .filter) }
        case .op(let o, let args):
            var c = encoder.container(keyedBy: Keys.self)
            try c.encode(o, forKey: .op)
            try c.encode(args, forKey: .args)
        case .cond(let i, let t, let e):
            var c = encoder.container(keyedBy: Keys.self)
            try c.encode(i, forKey: .if)
            try c.encode(t, forKey: .then)
            try c.encode(e, forKey: .else)
        }
    }
}

public extension Expr {
    /// 式が参照する entity id(agg の entity と filter.value の中)。画面のレコード取得対象の算出に使う。
    var referencedEntities: Set<String> {
        switch self {
        case .literal, .ref: return []
        case .agg(let a):
            return a.filters.reduce(into: [a.entity]) { $0.formUnion($1.valueExpr?.referencedEntities ?? []) }
        case .op(_, let args): return args.reduce(into: []) { $0.formUnion($1.referencedEntities) }
        case .cond(let i, let t, let e): return i.referencedEntities.union(t.referencedEntities).union(e.referencedEntities)
        }
    }
}
