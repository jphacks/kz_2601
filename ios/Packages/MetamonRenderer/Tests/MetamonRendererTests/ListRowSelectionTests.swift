import XCTest
@testable import MetamonRenderer

/// #1387: 一覧の選んでいる行を ↑↓ で移す・フォーカスが来た時・行が消えた時。
final class ListRowSelectionTests: XCTestCase {
    private let ids = ["a", "b", "c"]

    func testMovesOneRowAndStopsAtEnds() {
        XCTAssertEqual(ListRowSelection.moved(from: "a", .down, ids: ids), "b")
        XCTAssertEqual(ListRowSelection.moved(from: "b", .up, ids: ids), "a")
        XCTAssertEqual(ListRowSelection.moved(from: "a", .up, ids: ids), "a", "先頭で止まる")
        XCTAssertEqual(ListRowSelection.moved(from: "c", .down, ids: ids), "c", "末尾で止まる")
    }

    func testNothingSelectedStartsFromFirstOrLast() {
        XCTAssertEqual(ListRowSelection.moved(from: nil, .down, ids: ids), "a")
        XCTAssertEqual(ListRowSelection.moved(from: nil, .up, ids: ids), "c")
        XCTAssertEqual(ListRowSelection.moved(from: "gone", .down, ids: ids), "a", "消えた行からは先頭")
        XCTAssertNil(ListRowSelection.moved(from: nil, .down, ids: []))
    }

    func testFocusKeepsPreviousRowOrPicksFirst() {
        XCTAssertEqual(ListRowSelection.onFocus(current: "b", ids: ids), "b")
        XCTAssertEqual(ListRowSelection.onFocus(current: nil, ids: ids), "a")
        XCTAssertEqual(ListRowSelection.onFocus(current: "gone", ids: ids), "a")
        XCTAssertNil(ListRowSelection.onFocus(current: nil, ids: []))
    }

    func testDeletedRowIsUnselected() {
        XCTAssertEqual(ListRowSelection.pruned("b", ids: ids), "b")
        XCTAssertNil(ListRowSelection.pruned("gone", ids: ids))
        XCTAssertNil(ListRowSelection.pruned(nil, ids: ids))
    }

    // MARK: - 複数選択(#1398)

    private let order = ["a", "b", "c", "d"]

    func testCommandClickTogglesRows() {
        var s = ListRowMultiSelection()
        s.click("a", .plain, order: order)
        s.click("c", .toggle, order: order)
        XCTAssertEqual(s.ids, ["a", "c"])
        XCTAssertTrue(s.isMultiple)
        s.click("a", .toggle, order: order)
        XCTAssertEqual(s.ids, ["c"], "もう一度 ⌘ クリックで外す")
        XCTAssertFalse(s.isMultiple)
    }

    func testShiftClickSelectsRangeFromAnchor() {
        var s = ListRowMultiSelection()
        s.click("b", .plain, order: order)
        s.click("d", .extend, order: order)
        XCTAssertEqual(s.ordered(order), ["b", "c", "d"])
        s.click("a", .extend, order: order)
        XCTAssertEqual(s.ordered(order), ["a", "b"], "起点(b)は変えずに範囲を選び直す")
        XCTAssertEqual(s.cursor, "a")
    }

    func testPlainClickResetsToOneRow() {
        var s = ListRowMultiSelection()
        s.click("a", .plain, order: order)
        s.click("c", .extend, order: order)
        s.click("d", .plain, order: order)
        XCTAssertEqual(s.ids, ["d"])
        XCTAssertEqual(s.anchor, "d")
    }

    func testShiftArrowExtends() {
        var s = ListRowMultiSelection()
        s.select("b")
        s.extend(.down, order: order)
        s.extend(.down, order: order)
        XCTAssertEqual(s.ordered(order), ["b", "c", "d"])
        s.extend(.up, order: order)
        XCTAssertEqual(s.ordered(order), ["b", "c"])
    }

    func testPruneDropsDeletedRows() {
        var s = ListRowMultiSelection()
        s.click("a", .plain, order: order)
        s.click("c", .extend, order: order)
        s.prune(order: ["a", "d"])
        XCTAssertEqual(s.ids, ["a"])
        XCTAssertNil(s.cursor, "消えた行は起点にしない")
        XCTAssertEqual(s.single, "a", "残った 1 行は ⌘C・⌫ の対象")
    }

    /// ⌘ クリックで外した行を ⌘C・⌫・Return の対象にしない(残った 1 行が対象)
    func testCommandClickDeselectKeepsSelectedRowAsTarget() {
        var s = ListRowMultiSelection()
        s.click("a", .plain, order: order)
        s.click("b", .toggle, order: order)
        s.click("b", .toggle, order: order)
        XCTAssertEqual(s.ids, ["a"])
        XCTAssertEqual(s.single, "a")
        s.click("a", .toggle, order: order)
        XCTAssertNil(s.single, "何も選んでいなければ対象は無い")
    }
}
