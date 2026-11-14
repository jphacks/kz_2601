import XCTest
@testable import MetamonRenderer

/// Mac の表示 leaf の判定だけの関数(#941)を、パッケージの `swift test` で確かめる(Xcode のアプリのビルドが要らない)。
/// rating のキーは端で止まる・1 つずつ増やす / 減らす、calendar は幅から枠の高さ・高さから題の行数。
final class MacDisplayKeysTests: XCTestCase {
    // MARK: - rating: 増やす / 減らす

    func testArrowsStepOneAndStopAtEnds() {
        XCTAssertEqual(MacRatingKeys.value(after: .right, current: 2, count: 5), 3)
        XCTAssertEqual(MacRatingKeys.value(after: .up, current: 2, count: 5), 3, "→ と ↑ は同じ")
        XCTAssertEqual(MacRatingKeys.value(after: .left, current: 2, count: 5), 1)
        XCTAssertEqual(MacRatingKeys.value(after: .down, current: 2, count: 5), 1, "← と ↓ は同じ")
        XCTAssertEqual(MacRatingKeys.value(after: .right, current: 5, count: 5), 5, "最大で止まる")
        XCTAssertEqual(MacRatingKeys.value(after: .left, current: 0, count: 5), 0, "0(未評価)で止まる")
    }

    func testOutOfRangeCurrentIsClampedFirst() {
        XCTAssertEqual(MacRatingKeys.value(after: .right, current: 99, count: 5), 5, "最大より上は最大で止まる")
        XCTAssertEqual(MacRatingKeys.value(after: .left, current: -3, count: 5), 0, "負の今の値は 0 から見る")
        XCTAssertEqual(MacRatingKeys.value(after: .right, current: -3, count: 5), 1)
        XCTAssertEqual(MacRatingKeys.value(after: .left, current: 99, count: 5), 4, "最大より上は最大から 1 つ減る")
    }

    func testDigitsSetValueAndZeroOrDeleteClears() {
        XCTAssertEqual(MacRatingKeys.value(after: .digit(4), current: 1, count: 5), 4)
        XCTAssertEqual(MacRatingKeys.value(after: .digit(9), current: 1, count: 5), 5, "数より大きい数字は最大")
        XCTAssertEqual(MacRatingKeys.value(after: .digit(0), current: 3, count: 5), 0, "0 で未評価")
        XCTAssertEqual(MacRatingKeys.value(after: .delete, current: 3, count: 5), 0, "delete で未評価")
        XCTAssertNil(MacRatingKeys.value(after: .digit(12), current: 1, count: 5), "1 桁でない数字は使わない")
    }

    func testOtherKeysArePassedOn() {
        XCTAssertNil(MacRatingKeys.value(after: .space, current: 1, count: 5))
        XCTAssertNil(MacRatingKeys.value(after: .other, current: 1, count: 5))
    }

    /// VoiceOver の「増やす / 減らす」。使えないキーでも今の値を返す(nil にしない)
    func testAdjustedNeverReturnsNil() {
        XCTAssertEqual(MacRatingKeys.adjusted(increment: true, current: 3, count: 5), 4)
        XCTAssertEqual(MacRatingKeys.adjusted(increment: false, current: 3, count: 5), 2)
        XCTAssertEqual(MacRatingKeys.adjusted(increment: true, current: 5, count: 5), 5, "端で止まる")
        XCTAssertEqual(MacRatingKeys.adjusted(increment: false, current: 0, count: 5), 0)
        XCTAssertEqual(MacRatingKeys.adjusted(increment: true, current: 0, count: 0), 0, "星が無い時も落ちない")
    }

    // MARK: - calendar: 幅から高さ

    func testCellHeightGrowsWithWidthWithinLimits() {
        XCTAssertEqual(MacCalendarLayout.cellHeight(gridWidth: 300), 36, "狭い時は iOS と同じ 36pt")
        XCTAssertEqual(MacCalendarLayout.cellHeight(gridWidth: 336), 36, "ちょうど下限")
        XCTAssertEqual(MacCalendarLayout.cellHeight(gridWidth: 700), 75, "列の幅の 0.75 倍")
        XCTAssertEqual(MacCalendarLayout.cellHeight(gridWidth: 896), 96, "ちょうど上限")
        XCTAssertEqual(MacCalendarLayout.cellHeight(gridWidth: 2000), 96, "上限で止まる")
    }

    func testCellHeightFallsBackForBadWidth() {
        XCTAssertEqual(MacCalendarLayout.cellHeight(gridWidth: 0), 36, "幅が測れていない時は iOS と同じ")
        XCTAssertEqual(MacCalendarLayout.cellHeight(gridWidth: -10), 36)
        XCTAssertEqual(MacCalendarLayout.cellHeight(gridWidth: .nan), 36)
        XCTAssertEqual(MacCalendarLayout.cellHeight(gridWidth: .infinity), 36)
    }

    // MARK: - calendar: 高さから行数

    func testTitleLinesStartAt60AndCapAt3() {
        XCTAssertEqual(MacCalendarLayout.titleLines(cellHeight: 36), 0, "狭い枠は点だけ")
        XCTAssertEqual(MacCalendarLayout.titleLines(cellHeight: 59), 0, "60pt 未満は点だけ")
        XCTAssertEqual(MacCalendarLayout.titleLines(cellHeight: 60), 2, "60pt から題を出す")
        XCTAssertEqual(MacCalendarLayout.titleLines(cellHeight: 75), 2)
        XCTAssertEqual(MacCalendarLayout.titleLines(cellHeight: 76), 3)
        XCTAssertEqual(MacCalendarLayout.titleLines(cellHeight: 96), 3, "上限の枠でも 3 行まで")
        XCTAssertEqual(MacCalendarLayout.titleLines(cellHeight: 0), 0)
    }

    /// 幅 → 高さ → 行数 がつながっていること(広いウインドウでだけ題が出る)
    func testWidthToLinesEndToEnd() {
        func lines(width: CGFloat) -> Int { MacCalendarLayout.titleLines(cellHeight: MacCalendarLayout.cellHeight(gridWidth: width)) }
        XCTAssertEqual(lines(width: 400), 0, "iPhone 幅の一覧では点だけ")
        XCTAssertEqual(lines(width: 700), 2)
        XCTAssertEqual(lines(width: 1400), 3, "広いウインドウでは 3 行")
    }
}
