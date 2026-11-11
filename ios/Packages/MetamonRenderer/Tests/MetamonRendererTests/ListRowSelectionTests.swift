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
}
