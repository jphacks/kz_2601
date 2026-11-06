import Foundation
import XCTest
@testable import MetamonDSL

/// 正典スキーマ `docs/dsl/dsl.schema.json` と Swift の型がずれていないかを見る(#537)。
/// スキーマにキーを足して Swift 側(CodingKeys)に足し忘れると、そのキーは iOS で黙って捨てられるため。
///
/// - Component はフラットな props(全部品の props の和集合)で読むので、部品ごとに「スキーマのキー ⊆ Swift のキー」。
/// - Swift にだけあるキーは、理由付きで `swiftOnly*` に書いたものだけ許す。
final class SchemaSwiftKeysTests: XCTestCase {
    /// Swift だけが読み、スキーマに無いキー(キー → 理由)。今は無い。
    private let swiftOnlyComponentKeys: [String: String] = [:]
    private let swiftOnlyActionKeys: [String: String] = [:]

    // MARK: - 部品

    func testComponentTypesMatchSchema() throws {
        let schemaTypes = Set(try SchemaFixture.load().componentProps.keys)
        let swiftTypes = Set(ComponentType.allCases.filter { $0 != .unknown }.map(\.rawValue))
        XCTAssertEqual(schemaTypes.count, 73, "部品の数が 73 でない(7 Container + 66 Leaf・#351・matchList #507・flashcards #264・transcribeField / speakButton・drawingField・feed #348・fileImport)")
        assertSameSet(schema: schemaTypes, swift: swiftTypes, what: "部品の type",
                      swiftPlace: "ComponentType の case")
    }

    func testComponentPropsAreDecodable() throws {
        let props = try SchemaFixture.load().componentProps
        let swiftKeys = Component.codingKeyNames

        var missing: [String] = []
        for (type, keys) in props.sorted(by: { $0.key < $1.key }) {
            let lost = keys.subtracting(swiftKeys)
            if !lost.isEmpty { missing.append("  \(type): \(lost.sorted().joined(separator: ", "))") }
        }
        XCTAssert(missing.isEmpty, """
            スキーマにあるのに Swift が読まない部品の props(iOS で黙って捨てられる):
            \(missing.joined(separator: "\n"))
            → Component(Model/Component.swift)の Keys と init(from:) に足す。
            """)

        let allSchemaKeys = props.values.reduce(into: Set<String>()) { $0.formUnion($1) }
        let extra = swiftKeys.subtracting(allSchemaKeys).subtracting(swiftOnlyComponentKeys.keys)
        XCTAssert(extra.isEmpty, """
            Swift の Component が読むのに、どの部品のスキーマにも無いキー: \(extra.sorted().joined(separator: ", "))
            → スキーマに足すか、Swift から消すか、理由付きで swiftOnlyComponentKeys に書く。
            """)
        let staleAllow = Set(swiftOnlyComponentKeys.keys).intersection(allSchemaKeys)
        XCTAssert(staleAllow.isEmpty, "swiftOnlyComponentKeys にあるがスキーマに入ったキー(許可から消す): \(staleAllow.sorted())")
    }

    // MARK: - アクション

    func testActionTypesMatchSchema() throws {
        let schemaTypes = try SchemaFixture.load().actionTypes
        let swiftTypes = Set(ActionType.allCases.filter { $0 != .unknown }.map(\.rawValue))
        XCTAssertEqual(schemaTypes.count, 12, "アクションの数が 12 でない(#351)")
        assertSameSet(schema: schemaTypes, swift: swiftTypes, what: "アクションの type",
                      swiftPlace: "ActionType の case")
    }

    func testActionKeysMatchSchema() throws {
        let schemaKeys = try SchemaFixture.load().objectKeys("Action")
        assertSameSet(schema: schemaKeys, swift: Action.codingKeyNames, what: "Action のキー",
                      swiftPlace: "Action の Keys(Model/Action.swift)", allowSwiftOnly: swiftOnlyActionKeys)
    }

    // MARK: - entity / field など

    func testFieldKeysMatchSchema() throws {
        let schemaKeys = try SchemaFixture.load().objectKeys("Field")
        assertSameSet(schema: schemaKeys, swift: Field.codingKeyNames, what: "Field のキー",
                      swiftPlace: "Field の Keys(Model/AppDocument.swift)")
    }

    /// 自動合成の Decodable は stored property 名 = キーなので、Mirror で読む(@Lossy などの `_` は外す)。
    func testSynthesizedObjectKeysMatchSchema() throws {
        let schema = try SchemaFixture.load()
        let entity = try JSONDecoder().decode(Entity.self, from: Data(#"{"id":"e","name":"E","fields":[]}"#.utf8))
        let cases: [(def: String, swiftName: String, value: Any)] = [
            ("Entity", "Entity(Model/AppDocument.swift)", entity),
            ("ListItem", "ListItem(Model/Component.swift)", ListItem(title: "t")),
            ("Sort", "Sort(Model/Component.swift)", Sort(field: "f", direction: "asc")),
            ("TableColumn", "TableColumnSpec(Model/Component.swift)", TableColumnSpec(field: "f")),
            ("SensorThreshold", "SensorThreshold(Model/Component.swift)", SensorThreshold()),
            ("Activity", "ActivitySpec(Model/Action.swift)", ActivitySpec(title: "t")),
            ("Mark", "Mark(Records/RecordMark.swift)", Mark(id: "m", icon: "heart", label: "l")),
            ("PhotoAnalyze", "PhotoAnalyze(Records/PhotoReading.swift)", PhotoAnalyze(kind: .text, into: "f")),
            ("ImportColumn", "ImportColumn(Records/RecordImport.swift)", ImportColumn(column: "c", field: "f")),
            ("Notification", "NotificationSpec(Model/AppDocument.swift)", NotificationSpec(id: "n", title: "t")),
        ]
        for c in cases {
            assertSameSet(schema: try schema.objectKeys(c.def), swift: Self.storedPropertyNames(c.value),
                          what: "\(c.def) のキー", swiftPlace: "\(c.swiftName) の stored property")
        }
    }

    // MARK: - helpers

    private static func storedPropertyNames(_ value: Any) -> Set<String> {
        Set(Mirror(reflecting: value).children.compactMap { child in
            child.label.map { $0.hasPrefix("_") ? String($0.dropFirst()) : $0 }
        })
    }

    private func assertSameSet(schema: Set<String>, swift: Set<String>, what: String, swiftPlace: String,
                               allowSwiftOnly: [String: String] = [:], file: StaticString = #filePath, line: UInt = #line) {
        let missing = schema.subtracting(swift)
        XCTAssert(missing.isEmpty, """
            スキーマにあるのに Swift に無い\(what): \(missing.sorted().joined(separator: ", "))
            → \(swiftPlace) に足す(足さないと iOS で黙って捨てられる)。
            """, file: file, line: line)
        let extra = swift.subtracting(schema).subtracting(allowSwiftOnly.keys)
        XCTAssert(extra.isEmpty, """
            Swift にあるのにスキーマに無い\(what): \(extra.sorted().joined(separator: ", "))
            → スキーマ(docs/dsl/dsl.schema.json)に足すか、\(swiftPlace) から消す。
            """, file: file, line: line)
    }
}

/// スキーマ JSON を辞書のまま読む(JSON Schema の全機能は要らない: `$defs` の properties と enum / const だけ)。
private struct SchemaFixture {
    let defs: [String: Any]

    static func load() throws -> SchemaFixture {
        let data = try Data(contentsOf: ExamplesFixture.schemaURL)
        let root = try XCTUnwrap(try JSONSerialization.jsonObject(with: data) as? [String: Any])
        return SchemaFixture(defs: try XCTUnwrap(root["$defs"] as? [String: Any], "schema に $defs が無い"))
    }

    func def(_ name: String) throws -> [String: Any] {
        try XCTUnwrap(defs[name] as? [String: Any], "schema の $defs/\(name) が無い")
    }

    func objectKeys(_ name: String) throws -> Set<String> {
        Set(try XCTUnwrap(try def(name)["properties"] as? [String: Any], "$defs/\(name) に properties が無い").keys)
    }

    /// 部品 type → その部品のスキーマ上の props キー(type / id / area を含む)。
    var componentProps: [String: Set<String>] {
        get throws {
            var out: [String: Set<String>] = [:]
            let container = try def("Container")
            let containerKeys = try objectKeys("Container")
            for t in try Self.typeValues(container) { out[t] = containerKeys }

            let leafRefs = try XCTUnwrap(try def("Leaf")["anyOf"] as? [[String: Any]], "$defs/Leaf に anyOf が無い")
            for ref in leafRefs {
                let path = try XCTUnwrap(ref["$ref"] as? String, "Leaf の anyOf に $ref でない要素がある")
                let name = String(path.split(separator: "/").last ?? "")
                let leafDef = try def(name)
                let keys = try objectKeys(name)
                for t in try Self.typeValues(leafDef) {
                    XCTAssertNil(out[t], "部品 type \(t) がスキーマに 2 回ある")
                    out[t] = keys
                }
            }
            return out
        }
    }

    var actionTypes: Set<String> {
        get throws { Set(try Self.typeValues(try def("Action"))) }
    }

    /// `properties.type` の const か enum。
    private static func typeValues(_ def: [String: Any]) throws -> [String] {
        let type = try XCTUnwrap((def["properties"] as? [String: Any])?["type"] as? [String: Any], "properties.type が無い")
        if let c = type["const"] as? String { return [c] }
        return try XCTUnwrap(type["enum"] as? [String], "properties.type に const / enum が無い")
    }
}
