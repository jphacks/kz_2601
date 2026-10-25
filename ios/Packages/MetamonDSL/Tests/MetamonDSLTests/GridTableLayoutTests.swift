import XCTest
@testable import MetamonDSL

/// `gridTable`(#234): 見出しの生成・マスへの配置・空マスの初期値・今日の列。
final class GridTableLayoutTests: XCTestCase {
    private func timetable() throws -> (Component, Entity) {
        let dsl = try ExamplesFixture.load("timetable")
        let grid = try XCTUnwrap(dsl.screens.flatMap(\.allComponents).first { $0.type == .gridTable })
        return (grid, try XCTUnwrap(dsl.entity("lecture")))
    }

    private func component(_ json: String) throws -> Component {
        try JSONDecoder().decode(Component.self, from: Data(json.utf8))
    }

    private func entity(_ json: String) throws -> Entity {
        try JSONDecoder().decode(Entity.self, from: Data(json.utf8))
    }

    func testExampleDecodes() throws {
        let (grid, _) = try timetable()
        XCTAssertEqual(grid.columnField, "weekday")
        XCTAssertEqual(grid.rowField, "period")
        XCTAssertEqual(grid.rowRange, GridRange(from: 1, to: 5))
        XCTAssertEqual(grid.titleField, "subject")
        XCTAssertEqual(grid.onItemTap?.type, .navigate)
    }

    func testHeadersFromEnumOptionsAndRange() throws {
        let (grid, lecture) = try timetable()
        let layout = GridTableLayout(component: grid, entity: lecture, records: [])
        XCTAssertEqual(layout.columns.map(\.label), ["月", "火", "水", "木", "金"])
        XCTAssertEqual(layout.rows.map(\.label), ["1限", "2限", "3限", "4限", "5限"], "integer は rowRange + unit")
        XCTAssertEqual(layout.rows.first?.value, .number(1))
    }

    func testPlacesRecordsOnCells() throws {
        let (grid, lecture) = try timetable()
        let records = [
            Record(id: "a", data: ["subject": "線形代数", "weekday": "月", "period": 1]),
            Record(id: "b", data: ["subject": "英語", "weekday": "水", "period": 2]),
            Record(id: "c", data: ["subject": "英語演習", "weekday": "水", "period": .string("2")]), // 文字列の数字も同じマス
            Record(id: "d", data: ["subject": "土曜講座", "weekday": "土", "period": 1]),           // 列に無い曜日は載らない
            Record(id: "e", data: ["subject": "7限", "weekday": "月", "period": 7]),                // rowRange 外も載らない
        ]
        let layout = GridTableLayout(component: grid, entity: lecture, records: records)
        XCTAssertEqual(layout.records(at: .init(column: 0, row: 0)).map(\.id), ["a"])
        XCTAssertEqual(layout.records(at: .init(column: 2, row: 1)).map(\.id), ["b", "c"], "同じマスに複数(元の順)")
        XCTAssertTrue(layout.records(at: .init(column: 1, row: 0)).isEmpty)
        XCTAssertEqual(layout.cells.values.reduce(0) { $0 + $1.count }, 3)
    }

    func testEmptyCellPrefillCarriesColumnAndRowValues() throws {
        let (grid, lecture) = try timetable()
        let layout = GridTableLayout(component: grid, entity: lecture, records: [])
        XCTAssertEqual(layout.prefill(at: .init(column: 3, row: 2)), ["weekday": .string("木"), "period": .number(3)])
        // FieldCoding.normalize を通しても値が残る(フォームの draft に入る形)
        XCTAssertEqual(FieldCoding.normalize(.number(3), for: try XCTUnwrap(lecture.field("period"))), .number(3))
    }

    func testDefaultRowsWithoutRangeExpandToData() throws {
        let grid = try component(#"{"type":"gridTable","entity":"x","columnField":"day","rowField":"p","titleField":"t"}"#)
        let e = try entity(#"{"id":"x","name":"x","fields":[{"id":"t","name":"t","type":"string"},{"id":"day","name":"d","type":"enum","options":["月","火"]},{"id":"p","name":"p","type":"integer"}]}"#)
        XCTAssertEqual(GridTableLayout(component: grid, entity: e, records: []).rows.map(\.label), ["1", "2", "3", "4", "5", "6"])
        let late = [Record(id: "1", data: ["t": "x", "day": "月", "p": 8])]
        XCTAssertEqual(GridTableLayout(component: grid, entity: e, records: late).rows.count, 8, "記録が 8 限なら 8 行まで広げる")
    }

    func testEnumRowsAndExplicitColumns() throws {
        let grid = try component(#"{"type":"gridTable","entity":"s","columnField":"day","rowField":"slot","titleField":"who","columnValues":["土","日","土"]}"#)
        let e = try entity(#"{"id":"s","name":"s","fields":[{"id":"who","name":"w","type":"string"},{"id":"day","name":"d","type":"enum","options":["月","火","水","木","金","土","日"]},{"id":"slot","name":"s","type":"enum","options":["早番","遅番"]}]}"#)
        let layout = GridTableLayout(component: grid, entity: e, records: [Record(id: "1", data: ["who": "佐藤", "day": "日", "slot": "遅番"])])
        XCTAssertEqual(layout.columns.map(\.label), ["土", "日"], "columnValues の順・重複は 1 つに")
        XCTAssertEqual(layout.rows.map(\.label), ["早番", "遅番"])
        XCTAssertEqual(layout.records(at: .init(column: 1, row: 1)).map(\.id), ["1"])
        XCTAssertEqual(layout.prefill(at: .init(column: 0, row: 1)), ["day": .string("土"), "slot": .string("遅番")])
    }

    func testTodayColumn() {
        var cal = Calendar(identifier: .gregorian)
        cal.timeZone = TimeZone(identifier: "Asia/Tokyo")!
        let wed = ISO8601DateFormatter().date(from: "2026-10-28T03:00:00Z")! // 水曜(JST 12:00)
        XCTAssertEqual(GridTableLayout.todayColumn(labels: ["月", "火", "水", "木", "金"], now: wed, calendar: cal), 2)
        XCTAssertEqual(GridTableLayout.todayColumn(labels: ["月曜日", "火曜日", "水曜日"], now: wed, calendar: cal), 2)
        XCTAssertEqual(GridTableLayout.todayColumn(labels: ["Mon", "Tue", "Wed", "Thu"], now: wed, calendar: cal), 2)
        XCTAssertNil(GridTableLayout.todayColumn(labels: ["月", "火", "木"], now: wed, calendar: cal), "今日の曜日が無ければ強調なし")
        XCTAssertNil(GridTableLayout.todayColumn(labels: ["早番", "遅番", "水"], now: wed, calendar: cal), "曜日の表でなければ強調しない")
    }
}
