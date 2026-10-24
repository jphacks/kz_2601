import Foundation

/// 任意の JSON を表す値型。DSL の state / params / entity レコードの共通表現。
public enum JSONValue: Hashable, Codable, Sendable {
    case string(String)
    case number(Double)
    case bool(Bool)
    case null
    case array([JSONValue])
    case object([String: JSONValue])

    public init(from decoder: Decoder) throws {
        let c = try decoder.singleValueContainer()
        if c.decodeNil() { self = .null; return }
        if let b = try? c.decode(Bool.self) { self = .bool(b); return }
        if let n = try? c.decode(Double.self) { self = .number(n); return }
        if let s = try? c.decode(String.self) { self = .string(s); return }
        if let a = try? c.decode([JSONValue].self) { self = .array(a); return }
        if let o = try? c.decode([String: JSONValue].self) { self = .object(o); return }
        throw DecodingError.dataCorruptedError(in: c, debugDescription: "Unsupported JSON value")
    }

    public func encode(to encoder: Encoder) throws {
        var c = encoder.singleValueContainer()
        switch self {
        case .string(let s): try c.encode(s)
        case .number(let n): try c.encode(n)
        case .bool(let b): try c.encode(b)
        case .null: try c.encodeNil()
        case .array(let a): try c.encode(a)
        case .object(let o): try c.encode(o)
        }
    }

    // MARK: - Accessors

    public var stringValue: String? { if case .string(let s) = self { return s }; return nil }
    public var doubleValue: Double? { if case .number(let n) = self { return n }; return nil }
    public var boolValue: Bool? { if case .bool(let b) = self { return b }; return nil }
    public var arrayValue: [JSONValue]? { if case .array(let a) = self { return a }; return nil }
    public var objectValue: [String: JSONValue]? { if case .object(let o) = self { return o }; return nil }
    public var isNull: Bool { if case .null = self { return true }; return false }

    /// テンプレート埋め込み・Text 表示用。
    public var displayString: String {
        switch self {
        case .string(let s): return s
        case .number(let n):
            return (n == n.rounded() && abs(n) < 1e15) ? String(Int(n)) : String(n)
        case .bool(let b): return b ? "true" : "false"
        case .null: return ""
        case .array(let a): return a.map(\.displayString).joined(separator: ", ")
        case .object:
            return (try? JSONEncoder().encode(self)).flatMap { String(data: $0, encoding: .utf8) } ?? "{}"
        }
    }

    /// `visible` / `disabled` 判定用の真偽値化。
    public var truthy: Bool {
        switch self {
        case .null: return false
        case .bool(let b): return b
        case .number(let n): return n != 0
        case .string(let s): return !s.isEmpty && s != "false"
        case .array(let a): return !a.isEmpty
        case .object: return true
        }
    }

    // MARK: - Path access ("todos.0.title", "todos.count")

    public subscript(path path: String) -> JSONValue? {
        value(at: path.split(separator: ".").map(String.init))
    }

    public func value(at segments: [String]) -> JSONValue? {
        guard let first = segments.first else { return self }
        let rest = Array(segments.dropFirst())
        switch self {
        case .object(let o):
            return o[first]?.value(at: rest)
        case .array(let a):
            if first == "count" { return JSONValue.number(Double(a.count)).value(at: rest) }
            if let i = Int(first), a.indices.contains(i) { return a[i].value(at: rest) }
            return nil
        case .string(let s) where first == "count":
            return JSONValue.number(Double(s.count)).value(at: rest)
        default:
            return nil
        }
    }

    public mutating func set(_ segments: [String], to value: JSONValue) {
        guard let first = segments.first else { self = value; return }
        let rest = Array(segments.dropFirst())
        if let i = Int(first), var a = arrayValue ?? (isNull ? [] : nil) {
            while a.count <= i { a.append(.null) }
            a[i].set(rest, to: value)
            self = .array(a)
            return
        }
        var o = objectValue ?? [:]
        var child = o[first] ?? .null
        child.set(rest, to: value)
        o[first] = child
        self = .object(o)
    }

    public mutating func set(path: String, to value: JSONValue) {
        set(path.split(separator: ".").map(String.init), to: value)
    }
}

// MARK: - Literals (モック/テストの記述用)

extension JSONValue: ExpressibleByStringLiteral, ExpressibleByIntegerLiteral, ExpressibleByFloatLiteral,
    ExpressibleByBooleanLiteral, ExpressibleByArrayLiteral, ExpressibleByDictionaryLiteral, ExpressibleByNilLiteral {
    public init(stringLiteral value: String) { self = .string(value) }
    public init(integerLiteral value: Int) { self = .number(Double(value)) }
    public init(floatLiteral value: Double) { self = .number(value) }
    public init(booleanLiteral value: Bool) { self = .bool(value) }
    public init(arrayLiteral elements: JSONValue...) { self = .array(elements) }
    public init(dictionaryLiteral elements: (String, JSONValue)...) { self = .object(Dictionary(uniqueKeysWithValues: elements)) }
    public init(nilLiteral: ()) { self = .null }
}
