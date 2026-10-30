import XCTest
@testable import MetamonDSL

/// #261: table.groupBy + 集計列 = 店ごとの平均点・件数のランキング。
final class TableGroupingTests: XCTestCase {
    private let reviews = [
        Record(id: "1", data: ["shop": "麺屋こがね", "score": 5]),
        Record(id: "2", data: ["shop": "らーめん大将", "score": 3]),
        Record(id: "3", data: ["shop": "麺屋こがね", "score": 4]),
        Record(id: "4", data: ["shop": "中華そば 一番", "score": 5]),
        Record(id: "5", data: ["shop": "麺屋こがね ", "score": 4]),   // 末尾の空白は同じ店
        Record(id: "6", data: ["shop": "らーめん大将", "score": 4]),
        Record(id: "7", data: ["score": 1]),                          // 店が空 = どの行にも入れない
    ]

    private func table(_ json: String) throws -> Component {
        try JSONDecoder().decode(Component.self, from: Data(json.utf8))
    }

    private let columns = #"[{"field":"shop"},{"field":"score","aggregate":"avg"},{"field":"shop","aggregate":"count"}]"#

    /// 完了の条件: 同じ店に 3 件あるとランキングでは 1 行(平均点・件数)にまとまる。
    func testSameShopFoldsIntoOneRowSortedByAverage() throws {
        let c = try table(#"{"type":"table","entity":"review","groupBy":"shop","columns":\#(columns),"sort":{"field":"score","direction":"desc"}}"#)
        XCTAssertTrue(TableGrouping.isGrouped(c))
        let rows = TableGrouping.rows(c, records: reviews)
        XCTAssertEqual(rows.map(\.key), ["中華そば 一番", "麺屋こがね", "らーめん大将"])
        let kogane = try XCTUnwrap(rows.first { $0.key == "麺屋こがね" })
        XCTAssertNil(kogane.values[0])
        XCTAssertEqual(try XCTUnwrap(kogane.values[1]), 13.0 / 3, accuracy: 1e-9)
        XCTAssertEqual(kogane.values[2], 3)
    }

    /// sort.field が groupBy の field = 件数列(集計列を優先)。同点はグループ名順。sort 省略 = 最初の集計列の降順。
    func testSortByCountAndDefault() throws {
        let byCount = try table(#"{"type":"table","entity":"review","groupBy":"shop","columns":\#(columns),"sort":{"field":"shop","direction":"desc"}}"#)
        XCTAssertEqual(TableGrouping.rows(byCount, records: reviews).map(\.key), ["麺屋こがね", "らーめん大将", "中華そば 一番"])
        let byDefault = try table(#"{"type":"table","entity":"review","groupBy":"shop","columns":\#(columns),"limit":2}"#)
        XCTAssertEqual(TableGrouping.rows(byDefault, records: reviews).map(\.key), ["中華そば 一番", "麺屋こがね"])
        // groupBy の列しか無い sort でも件数列が無ければグループ名順
        let byName = try table(#"{"type":"table","entity":"review","groupBy":"shop","columns":[{"field":"shop"},{"field":"score","aggregate":"sum"}],"sort":{"field":"shop","direction":"asc"}}"#)
        let abc = [Record(id: "1", data: ["shop": "B", "score": 1]), Record(id: "2", data: ["shop": "C", "score": 9]), Record(id: "3", data: ["shop": "A", "score": 5])]
        XCTAssertEqual(TableGrouping.rows(byName, records: abc).map(\.key), ["A", "B", "C"])
    }

    /// avg / min / max は数値が無ければ nil(「—」)、sum / count は 0。
    func testEmptyNumbers() throws {
        let c = try table(#"{"type":"table","entity":"review","groupBy":"shop","columns":[{"field":"shop"},{"field":"score","aggregate":"avg"},{"field":"score","aggregate":"sum"},{"field":"shop","aggregate":"count"}]}"#)
        let rows = TableGrouping.rows(c, records: [Record(id: "1", data: ["shop": "A"])])
        XCTAssertEqual(rows.count, 1)
        XCTAssertNil(rows[0].values[1])
        XCTAssertEqual(rows[0].values[2], 0)
        XCTAssertEqual(rows[0].values[3], 1)
    }

    /// 寛容さ: groupBy だけ・未知の集計だけならグループ表にしない(従来のレコード表)。列の未知キーは無視して読める。
    func testLenientDecoding() throws {
        let noAgg = try table(#"{"type":"table","entity":"review","groupBy":"shop","columns":[{"field":"shop"}]}"#)
        XCTAssertFalse(TableGrouping.isGrouped(noAgg))
        let future = try table(#"{"type":"table","entity":"review","groupBy":"shop","columns":[{"field":"shop","futureKey":1},{"field":"score","aggregate":"median"}]}"#)
        XCTAssertFalse(TableGrouping.isGrouped(future))
        XCTAssertEqual(future.columns?.count, 2)
        XCTAssertEqual(future.columns?[1].aggregate, "median")
    }

    /// 正典サンプル ranking.json が groupBy と集計列を保ったまま読める。
    func testRankingExample() throws {
        let dsl = try ExamplesFixture.load("ranking")
        let home = try XCTUnwrap(dsl.screens.first { $0.id == "home" })
        let tbl = try XCTUnwrap(home.components.first?.children.first)
        XCTAssertEqual(tbl.type, .table)
        XCTAssertEqual(tbl.groupBy, "shop")
        XCTAssertEqual(tbl.columns?.map(\.aggregate), [nil, "avg", "count"])
        XCTAssertTrue(TableGrouping.isGrouped(tbl))
    }
}
