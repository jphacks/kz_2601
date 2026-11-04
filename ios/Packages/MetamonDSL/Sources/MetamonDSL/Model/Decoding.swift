import Foundation

/// 要素単位でデコード失敗を無視する配列。壊れた子要素1つで画面全体を落とさないための防波堤。
@propertyWrapper
public struct Lossy<Element: Decodable>: Decodable {
    public var wrappedValue: [Element]

    public init(wrappedValue: [Element] = []) { self.wrappedValue = wrappedValue }

    public init(from decoder: Decoder) throws {
        var c = try decoder.unkeyedContainer()
        var out: [Element] = []
        while !c.isAtEnd {
            if let v = try? c.decode(Element.self) {
                out.append(v)
            } else if (try? c.decode(JSONValue.self)) == nil {
                break // 進められない = 配列自体が壊れている
            }
        }
        wrappedValue = out
    }
}

extension Lossy: Sendable where Element: Sendable {}

public extension KeyedDecodingContainer {
    /// キー欠落時は空配列(`@Lossy var children: [Node]` を optional にしなくて済む)。
    func decode<T>(_ type: Lossy<T>.Type, forKey key: Key) throws -> Lossy<T> {
        try decodeIfPresent(type, forKey: key) ?? Lossy()
    }
}

/// 正の整数だけを受け、型違い・0 以下・欠落は nil にする(#482 `Entity.retentionDays`)。壊れた値で entity ごと落とさない。
@propertyWrapper
public struct LossyPositiveInt: Decodable, Sendable {
    public var wrappedValue: Int?

    public init(wrappedValue: Int? = nil) { self.wrappedValue = wrappedValue }

    public init(from decoder: Decoder) throws {
        let c = try? decoder.singleValueContainer()
        let n = (try? c?.decode(Int.self)) ?? (try? c?.decode(Double.self)).flatMap { $0.isFinite && $0 == $0.rounded() ? Int(exactly: $0) : nil }
        wrappedValue = n.flatMap { $0 > 0 ? $0 : nil }
    }
}

public extension KeyedDecodingContainer {
    func decode(_ type: LossyPositiveInt.Type, forKey key: Key) throws -> LossyPositiveInt {
        (try? decodeIfPresent(type, forKey: key)) ?? LossyPositiveInt()
    }
}

/// 未知の enum 文字列を fallback に丸める(バックエンドが新しい値を追加しても落ちない)。
public enum Lenient {
    public static func decode<E: RawRepresentable>(_: E.Type, from decoder: Decoder, fallback: E) -> E where E.RawValue == String {
        guard let raw = try? decoder.singleValueContainer().decode(String.self) else { return fallback }
        return E(rawValue: raw) ?? fallback
    }
}
