import Foundation
import XCTest
@testable import MetamonDSL

/// 計算式の共有テストデータ `docs/dsl/conformance/expr/*.json`(#545)を iOS の評価器で回す。
/// iOS が正: `EXPR_CONFORMANCE_UPDATE=1 swift test --filter ExprConformanceTests` で各ケースの `expected` を
/// iOS の評価結果で書き直す。Go(`backend/pkg/dsl/expreval`)は同じファイルを読んで一致を確かめる。
final class ExprConformanceTests: XCTestCase {
    static let dir = ExamplesFixture.repoRoot.appending(path: "docs/dsl/conformance/expr")

    private struct Defaults: Decodable {
        var now: String
        var timeZone: String
        var entities: [Entity]?
        var records: [String: [Record]]?
    }

    private struct RowSpec: Decodable {
        var entity: String
        var record: Record
    }

    private struct Case: Decodable {
        var name: String
        var expr: Expr
        var now: String?
        var timeZone: String?
        var row: RowSpec?
        var entities: [Entity]?
        var records: [String: [Record]]?
        var expected: JSONValue?
    }

    private struct File: Decodable {
        var defaults: Defaults
        var cases: [Case]
    }

    func testConformance() throws {
        let update = ProcessInfo.processInfo.environment["EXPR_CONFORMANCE_UPDATE"] == "1"
        let files = try FileManager.default.contentsOfDirectory(at: Self.dir, includingPropertiesForKeys: nil)
            .filter { $0.pathExtension == "json" }
            .sorted { $0.lastPathComponent < $1.lastPathComponent }
        XCTAssertFalse(files.isEmpty, "no case files in \(Self.dir.path)")

        let savedTZ = FieldCoding.dateFormatter.timeZone
        defer { FieldCoding.dateFormatter.timeZone = savedTZ }

        var total = 0
        for url in files {
            let data = try Data(contentsOf: url)
            let file = try JSONDecoder().decode(File.self, from: data)
            var tree = try XCTUnwrap(JSONSerialization.jsonObject(with: data) as? [String: Any])
            var rawCases = try XCTUnwrap(tree["cases"] as? [[String: Any]])
            for (i, c) in file.cases.enumerated() {
                let got = try encode(evaluate(c, defaults: file.defaults), timeZone: c.timeZone ?? file.defaults.timeZone)
                total += 1
                if update {
                    rawCases[i]["expected"] = got.any
                } else {
                    guard let want = c.expected else {
                        XCTFail("\(url.lastPathComponent) \(c.name): expected is missing (run with EXPR_CONFORMANCE_UPDATE=1)")
                        continue
                    }
                    XCTAssertEqual(got, want, "\(url.lastPathComponent) \(c.name)")
                }
            }
            if update {
                tree["cases"] = rawCases
                try (ConformanceJSON.render(tree) + "\n").write(to: url, atomically: true, encoding: .utf8)
            }
        }
        XCTAssertGreaterThan(total, 0)
    }

    private func evaluate(_ c: Case, defaults: Defaults) throws -> ExprValue? {
        let tz = try XCTUnwrap(TimeZone(identifier: c.timeZone ?? defaults.timeZone), "\(c.name): unknown timeZone")
        let now = try XCTUnwrap(ISO8601DateFormatter().date(from: c.now ?? defaults.now), "\(c.name): bad now")
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = tz
        // 評価器の外の静的フォーマッタ("YYYY-MM-DD" の読み書き)も端末 TZ を使うので、ケースの TZ に合わせる
        FieldCoding.dateFormatter.timeZone = tz
        let entities = c.entities ?? defaults.entities ?? []
        let records = c.records ?? defaults.records ?? [:]
        let ev = ExprEvaluator(entities: entities, records: { records[$0] ?? [] }, now: now, calendar: calendar)
        let row = c.row.map { r in ExprEvaluator.Row(entity: entities.first { $0.id == r.entity }, record: r.record) }
        return ev.evaluate(c.expr, row: row)
    }

    /// 評価結果 → `{"type", "value", "text"}`(nil は `{"type":"nil"}`)。日付はケースの TZ の RFC 3339。
    private func encode(_ v: ExprValue?, timeZone: String) throws -> JSONValue {
        guard let v else { return ["type": "nil"] }
        let text = JSONValue.string(v.text)
        switch v {
        case .number(let n):
            let value: JSONValue = n.isNaN ? "nan" : n.isInfinite ? (n > 0 ? "inf" : "-inf") : .number(n)
            return ["type": "number", "value": value, "text": text]
        case .string(let s): return ["type": "string", "value": .string(s), "text": text]
        case .bool(let b): return ["type": "bool", "value": .bool(b), "text": text]
        case .date(let d):
            let f = ISO8601DateFormatter()
            f.timeZone = try XCTUnwrap(TimeZone(identifier: timeZone))
            return ["type": "date", "value": .string(f.string(from: d)), "text": text]
        }
    }
}

private extension JSONValue {
    /// JSONSerialization の木に戻す(update で書き戻す用)。
    var any: Any {
        switch self {
        case .string(let s): return s
        case .number(let n): return NSNumber(value: n)
        case .bool(let b): return NSNumber(value: b)
        case .null: return NSNull()
        case .array(let a): return a.map(\.any)
        case .object(let o): return o.mapValues(\.any)
        }
    }
}

/// update で書くファイルの整形。キーは読みやすい順、短い値は 1 行にまとめる(差分が読める形)。
enum ConformanceJSON {
    static let keyOrder = ["description", "defaults", "cases", "id", "name", "note", "now", "timeZone", "expr", "row", "entity", "record",
                           "entities", "records", "expected", "type", "value", "text", "data",
                           "op", "args", "agg", "field", "by", "filter", "ref", "if", "then", "else"]
    static let inlineWidth = 120

    static func render(_ v: Any, indent: Int = 0) -> String {
        let flat = compact(v)
        if flat.count + indent <= inlineWidth || !(v is [String: Any] || v is [Any]) { return flat }
        let pad = String(repeating: "  ", count: indent + 1)
        let end = String(repeating: "  ", count: indent)
        if let o = v as? [String: Any] {
            let body = sortedKeys(o).map { pad + quote($0) + ": " + render(o[$0]!, indent: indent + 1) }
            return "{\n" + body.joined(separator: ",\n") + "\n" + end + "}"
        }
        let a = v as! [Any]
        return "[\n" + a.map { pad + render($0, indent: indent + 1) }.joined(separator: ",\n") + "\n" + end + "]"
    }

    static func compact(_ v: Any) -> String {
        switch v {
        case let o as [String: Any]:
            return "{" + sortedKeys(o).map { quote($0) + ": " + compact(o[$0]!) }.joined(separator: ", ") + "}"
        case let a as [Any]:
            return "[" + a.map(compact).joined(separator: ", ") + "]"
        case let s as String:
            return quote(s)
        case let n as NSNumber:
            if CFGetTypeID(n) == CFBooleanGetTypeID() { return n.boolValue ? "true" : "false" }
            let d = n.doubleValue
            return d == d.rounded() && abs(d) < 1e15 ? String(Int(d)) : String(d)
        case is NSNull:
            return "null"
        default:
            return "null"
        }
    }

    static func sortedKeys(_ o: [String: Any]) -> [String] {
        o.keys.sorted { a, b in
            let x = keyOrder.firstIndex(of: a) ?? Int.max, y = keyOrder.firstIndex(of: b) ?? Int.max
            return x != y ? x < y : a < b
        }
    }

    static func quote(_ s: String) -> String {
        var out = "\""
        for u in s.unicodeScalars {
            switch u {
            case "\"": out += "\\\""
            case "\\": out += "\\\\"
            case "\n": out += "\\n"
            case "\r": out += "\\r"
            case "\t": out += "\\t"
            case _ where u.value < 0x20 || (0x300...0x36F).contains(u.value) || (0x200B...0x200F).contains(u.value)
                || (0xFE00...0xFE0F).contains(u.value) || (0x1F3FB...0x1F3FF).contains(u.value) || u.value == 0x3099:
                // 制御文字と、見た目で分からない結合文字・ZWJ・異体字セレクタは \u で残す
                out += String(format: "\\u%04x", u.value)
            default: out.unicodeScalars.append(u)
            }
        }
        return out + "\""
    }
}
