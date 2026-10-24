import Foundation

/// 描画中のスコープ。画面 params と、list 行の item/index を持ち回る。
struct RenderContext {
    var params: [String: JSONValue] = [:]
    var item: JSONValue?
    var index: Int?

    func with(item: JSONValue, index: Int) -> RenderContext {
        var c = self
        c.item = item
        c.index = index
        return c
    }
}

/// `{{state.todos.count}}` 形式のテンプレート展開。
/// ルート: `state` / `item` / `index` / `params` / `user`。ルート省略時は state。
enum Interpolator {
    struct Scope {
        var state: JSONValue
        var user: JSONValue
        var ctx: RenderContext
    }

    /// 文字列全体が単一式なら型付き値を返し、文中埋め込みなら文字列連結する。
    static func resolve(_ template: String, in scope: Scope) -> JSONValue {
        let trimmed = template.trimmingCharacters(in: .whitespaces)
        if trimmed.hasPrefix("{{"), trimmed.hasSuffix("}}"), !trimmed.dropFirst(2).contains("{{") {
            let expr = trimmed.dropFirst(2).dropLast(2).trimmingCharacters(in: .whitespaces)
            return lookup(expr, in: scope) ?? .null
        }
        guard template.contains("{{") else { return .string(template) }

        var out = ""
        var rest = Substring(template)
        while let open = rest.range(of: "{{") {
            out += rest[..<open.lowerBound]
            rest = rest[open.upperBound...]
            guard let close = rest.range(of: "}}") else { out += "{{"; break }
            let expr = rest[..<close.lowerBound].trimmingCharacters(in: .whitespaces)
            out += (lookup(expr, in: scope) ?? .null).displayString
            rest = rest[close.upperBound...]
        }
        out += rest
        return .string(out)
    }

    /// JSONValue 内の全文字列を再帰的に展開(action の data / params 用)。
    static func resolve(_ value: JSONValue, in scope: Scope) -> JSONValue {
        switch value {
        case .string(let s): return resolve(s, in: scope)
        case .array(let a): return .array(a.map { resolve($0, in: scope) })
        case .object(let o): return .object(o.mapValues { resolve($0, in: scope) })
        default: return value
        }
    }

    static func lookup(_ expr: String, in scope: Scope) -> JSONValue? {
        var segs = expr.split(separator: ".").map(String.init)
        guard let root = segs.first else { return nil }
        segs.removeFirst()
        switch root {
        case "state": return scope.state.value(at: segs)
        case "item": return scope.ctx.item?.value(at: segs)
        case "index": return scope.ctx.index.map { .number(Double($0)) }
        case "params": return JSONValue.object(scope.ctx.params).value(at: segs)
        case "user": return scope.user.value(at: segs)
        default: return scope.state.value(at: [root] + segs)
        }
    }

    /// `state.foo` と `foo` を同一視するための正規化。
    static func statePath(_ path: String) -> String {
        path.hasPrefix("state.") ? String(path.dropFirst(6)) : path
    }
}
