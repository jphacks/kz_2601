import Foundation

/// 要素単位でデコード失敗を無視する配列。壊れた子要素1つで画面全体を落とさないための防波堤。
@propertyWrapper
struct Lossy<Element: Decodable>: Decodable {
    var wrappedValue: [Element]

    init(wrappedValue: [Element] = []) { self.wrappedValue = wrappedValue }

    init(from decoder: Decoder) throws {
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

extension KeyedDecodingContainer {
    /// キー欠落時は空配列(`@Lossy var children: [Node]` を optional にしなくて済む)。
    func decode<T>(_ type: Lossy<T>.Type, forKey key: Key) throws -> Lossy<T> {
        try decodeIfPresent(type, forKey: key) ?? Lossy()
    }
}

/// 未知の enum 文字列を fallback に丸める(バックエンドが新しい値を追加しても落ちない)。
enum Lenient {
    static func decode<E: RawRepresentable>(_: E.Type, from decoder: Decoder, fallback: E) -> E where E.RawValue == String {
        guard let raw = try? decoder.singleValueContainer().decode(String.self) else { return fallback }
        return E(rawValue: raw) ?? fallback
    }
}
