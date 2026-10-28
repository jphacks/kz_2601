import XCTest
@testable import MetamonDSL

/// #255: 共有 entity の印(marks)・`$marks.<id>`・「自分が付けたもの」の filter。
final class RecordMarkTests: XCTestCase {
    private let recipes: [Record] = [
        Record(id: "1", data: ["title": "肉じゃが"], marks: ["save": MarkState(count: 3, mine: false)]),
        Record(id: "2", data: ["title": "カレー"], marks: ["save": MarkState(count: 1, mine: true)]),
        Record(id: "3", data: ["title": "卵焼き"]), // サーバーが marks を返していない
    ]

    func testRecipesExampleDecodesMarks() throws {
        let dsl = try ExamplesFixture.load("recipes")
        let entity = try XCTUnwrap(dsl.entity("recipes"))
        XCTAssertEqual(entity.marks, [Mark(id: "save", icon: "bookmark", label: "保存")])
        let home = try XCTUnwrap(dsl.screen("home")?.allComponents.first { $0.type == .list })
        XCTAssertEqual(home.item?.mark, "save")
        let saved = try XCTUnwrap(dsl.screen("saved")?.allComponents.first { $0.type == .list })
        XCTAssertEqual(saved.filters.count, 1)
        XCTAssertTrue(saved.filters[0].isMe, "{\"me\":true} が Expr として捨てられない")
        XCTAssertEqual(saved.filters[0].field, "$marks.save")
    }

    func testSchemaIconsHaveSymbols() throws {
        let data = try Data(contentsOf: ExamplesFixture.schemaURL)
        let root = try XCTUnwrap(try JSONSerialization.jsonObject(with: data) as? [String: Any])
        let mark = try XCTUnwrap((root["$defs"] as? [String: Any])?["Mark"] as? [String: Any])
        let icons = try XCTUnwrap(((mark["properties"] as? [String: Any])?["icon"] as? [String: Any])?["enum"] as? [String])
        for icon in icons {
            let m = Mark(id: "x", icon: icon, label: "x")
            XCTAssertTrue(icon == "heart" || m.systemImage(on: false) != "heart", "icon \(icon) が heart に落ちている")
            XCTAssertEqual(m.systemImage(on: true), m.systemImage(on: false) + ".fill")
        }
    }

    func testRecordDecodesMarksAndSubscript() throws {
        let json = #"{"id":"r1","data":{"title":"肉じゃが"},"marks":{"save":{"count":2,"mine":true}}}"#
        let r = try JSONDecoder().decode(Record.self, from: Data(json.utf8))
        XCTAssertEqual(r.markState("save"), MarkState(count: 2, mine: true))
        XCTAssertEqual(r["$marks.save"], .number(2))
        XCTAssertEqual(recipes[2]["$marks.save"], .number(0), "無ければ 0 件")
    }

    func testFieldAndSort() throws {
        let entity = try XCTUnwrap(ExamplesFixture.load("recipes").entity("recipes"))
        XCTAssertEqual(entity.field("$marks.save")?.type, .integer)
        XCTAssertEqual(entity.field("$marks.save")?.name, "保存")
        XCTAssertNil(entity.field("$marks.like"), "宣言していない印")
        let sorted = Aggregator.sorted(recipes, by: Sort(field: "$marks.save", direction: "desc"), entity: entity)
        XCTAssertEqual(sorted.map(\.id), ["1", "2", "3"])
    }

    func testMarkedByMeFilter() {
        let me = Filter(field: "$marks.save", op: "eq", value: Filter.meValue).resolvingMe(uid: "u1")
        XCTAssertEqual(recipes.filter { Aggregator.matches(me, record: $0, field: nil) }.map(\.id), ["2"])
        let notYet = Filter(field: "$marks.save", op: "neq", value: Filter.meValue).resolvingMe(uid: nil)
        XCTAssertEqual(recipes.filter { Aggregator.matches(notYet, record: $0, field: nil) }.map(\.id), ["1", "3"])
    }

    func testToggledNeverBelowZero() {
        XCTAssertEqual(MarkState(count: 0, mine: true).toggled(), MarkState(count: 0, mine: false))
        XCTAssertEqual(MarkState(count: 2, mine: false).toggled(), MarkState(count: 3, mine: true))
    }
}
