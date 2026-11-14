import XCTest
@testable import MetamonDSL

/// 設定値の置き場(#267 `Entity.single` と式 `{"setting": ...}`)。値の決まりは共有テスト `docs/dsl/conformance/expr/setting.json`。
final class SingleEntityTests: XCTestCase {
    private func entity(_ extra: String = "") throws -> Entity {
        let json = #"{"id":"settings","name":"設定","fields":[{"id":"cycle_days","name":"周期","type":"integer","defaultValue":28}]"# + extra + "}"
        return try JSONDecoder().decode(Entity.self, from: Data(json.utf8))
    }

    func testDecodesSingleLeniently() throws {
        XCTAssertTrue(try entity(#","single":true"#).single)
        XCTAssertFalse(try entity().single)
        XCTAssertFalse(try entity(#","single":false"#).single)
        // 壊れた値は entity ごと落とさず false
        XCTAssertFalse(try entity(#","single":"yes""#).single)
    }

    func testSettingExprRoundTrips() throws {
        let e = try JSONDecoder().decode(Expr.self, from: Data(#"{"setting":"cycle_days"}"#.utf8))
        XCTAssertEqual(e, .setting("cycle_days"))
        XCTAssertEqual(String(decoding: try JSONEncoder().encode(e), as: UTF8.self), #"{"setting":"cycle_days"}"#)
        XCTAssertEqual(e.referencedEntities, [])
        // 入れ子でも読める
        let nested = try JSONDecoder().decode(Expr.self, from: Data(#"{"op":"addDays","args":[{"op":"today","args":[]},{"setting":"cycle_days"}]}"#.utf8))
        guard case .op("addDays", let args) = nested else { return XCTFail() }
        XCTAssertEqual(args[1], .setting("cycle_days"))
    }

    func testPickNewestRecord() {
        XCTAssertNil(SingleRecord.pick([]))
        let a = Record(id: "a", data: [:], createdAt: "2026-10-01T00:00:00Z")
        let b = Record(id: "b", data: [:], createdAt: "2026-10-05T00:00:00Z")
        XCTAssertEqual(SingleRecord.pick([b, a])?.id, "b")
        XCTAssertEqual(SingleRecord.pick([a, b])?.id, "b")
        // 同じ(どちらも無い)なら後の方
        XCTAssertEqual(SingleRecord.pick([Record(id: "x", data: [:]), Record(id: "y", data: [:])])?.id, "y")
    }

    func testSettingReadsDefaultThenSaved() throws {
        let settings = try entity(#","single":true"#)
        var rows: [Record] = []
        let make = { ExprEvaluator(entities: [settings], records: { $0 == "settings" ? rows : [] }) }
        XCTAssertEqual(make().evaluate(.setting("cycle_days")), .number(28))
        rows = [Record(id: "s", data: ["cycle_days": .number(30)])]
        XCTAssertEqual(make().evaluate(.setting("cycle_days")), .number(30))
        XCTAssertNil(make().evaluate(.setting("nope")))
        XCTAssertNil(make().evaluate(.setting("$createdAt")))
        // single でない entity の field は読まない
        let plain = try entity()
        XCTAssertNil(ExprEvaluator(entities: [plain], records: { _ in rows }).evaluate(.setting("cycle_days")))
    }

    func testEntitiesNeededAlwaysHasSingles() throws {
        let json = #"""
        {"schemaVersion":1,"app":{"name":"x","voiceEntry":"floating"},"initialScreen":"home",
         "entities":[{"id":"logs","name":"記録","fields":[{"id":"d","name":"日","type":"date"}]},
                     {"id":"settings","name":"設定","single":true,"fields":[{"id":"goal","name":"目標","type":"number"}]}],
         "screens":[{"id":"home","title":"h","components":[{"type":"stat","label":"目標","value":{"setting":"goal"}}]}]}
        """#
        let dsl = try JSONDecoder().decode(AppDSL.self, from: Data(json.utf8))
        XCTAssertEqual(dsl.entitiesNeeded(by: dsl.screens[0]), ["settings"])
    }
}
