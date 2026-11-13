import XCTest
import MetamonDSL
@testable import MetamonRenderer
@testable import Metamon

/// table / gridTable を Mac の Table で描く試作(#2326): フラグの既定と、列の見出しで並べ替える値。
@MainActor
final class MacTableExperimentTests: XCTestCase {
    func testFlagIsOffByDefault() throws {
        let suite = "test-macTable-\(UUID().uuidString)"
        let defaults = try XCTUnwrap(UserDefaults(suiteName: suite))
        defer { defaults.removePersistentDomain(forName: suite) }
        XCTAssertFalse(MacTableExperiment.isEnabled(defaults), "既定は今の描き方")
        defaults.set(true, forKey: MacTableExperiment.defaultsKey)
        XCTAssertTrue(MacTableExperiment.isEnabled(defaults), "DEBUG のつまみで on")
    }

    private func row(_ id: String, _ texts: [String], _ keys: [TableSortKey]) -> TableLeaf.CellRow {
        TableLeaf.CellRow(id: id, texts: texts, keys: keys)
    }

    func testSortsNumbersNumericallyAndEmptyLast() {
        let rows = [
            row("a", ["¥1,000"], [.number(1000)]),
            row("b", [""], [.none]),
            row("c", ["¥200"], [.number(200)]),
        ]
        XCTAssertEqual(MacTableSort.sorted(rows, by: [MacTableSort(column: 0)]).map(\.id), ["c", "a", "b"],
                       "表示の文字(¥1,000 < ¥200)でなく数で比べる")
        XCTAssertEqual(MacTableSort.sorted(rows, by: [MacTableSort(column: 0, order: .reverse)]).map(\.id), ["a", "c", "b"],
                       "降順でも値の無い行は最後")
        XCTAssertEqual(MacTableSort.sorted(rows, by: []).map(\.id), ["a", "b", "c"], "並べ替えていなければ今の並び")
    }

    func testSortsTextLikeFinder() {
        let rows = [row("a", ["item10"], [.text("item10")]), row("b", ["item2"], [.text("item2")])]
        XCTAssertEqual(MacTableSort.sorted(rows, by: [MacTableSort(column: 0)]).map(\.id), ["b", "a"])
    }

    func testSortKeyFromValue() {
        XCTAssertEqual(TableSortKey.of(.number(3), display: "3 回"), .number(3))
        XCTAssertEqual(TableSortKey.of(.string("ラーメン"), display: "ラーメン"), .text("ラーメン"))
        XCTAssertEqual(TableSortKey.of(nil, display: ""), TableSortKey.none)
    }

    func testCopyIsTabSeparatedWithHeader() {
        let columns = [MacTableColumn(index: 0, title: "お店", field: "shop", numeric: false),
                       MacTableColumn(index: 1, title: "点数", field: "score", numeric: true)]
        let text = MacTableCopy.text(columns: columns, rows: [row("a", ["A店", "4"], [])])
        XCTAssertEqual(text, "お店\t点数\nA店\t4")
    }

    func testColumnIdsAreDistinctForSameField() {
        let a = MacTableColumn(index: 0, title: "点数", field: "score", numeric: true)
        let b = MacTableColumn(index: 1, title: "点数の平均", field: "score", numeric: true)
        XCTAssertNotEqual(a.id, b.id, "同じ field の列も別の列として並べ替える")
    }
}
