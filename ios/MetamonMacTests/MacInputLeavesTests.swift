import XCTest
import AppKit
@testable import MetamonRenderer

/// Mac の入力 leaf の決まり(#942)。
final class MacInputLeavesTests: XCTestCase {
    // MARK: numberField の文字の絞り込み

    func testFilterDropsNonDigits() {
        XCTAssertEqual(MacNumberInput.filter("1a2b3", allowsDecimal: false, allowsNegative: false), "123")
        XCTAssertEqual(MacNumberInput.filter("¥1,000円", allowsDecimal: false, allowsNegative: false), "1000")
    }

    func testFilterDecimalAndMinusByFieldType() {
        XCTAssertEqual(MacNumberInput.filter("-1.5", allowsDecimal: false, allowsNegative: false), "15")
        XCTAssertEqual(MacNumberInput.filter("-1.5", allowsDecimal: true, allowsNegative: true), "-1.5")
        XCTAssertEqual(MacNumberInput.filter("1.2.3", allowsDecimal: true, allowsNegative: true), "1.23")
        XCTAssertEqual(MacNumberInput.filter("1-2", allowsDecimal: true, allowsNegative: true), "12")
    }

    func testFilterConvertsFullWidth() {
        XCTAssertEqual(MacNumberInput.filter("１２．５", allowsDecimal: true, allowsNegative: false), "12.5")
        XCTAssertEqual(MacNumberInput.filter("－３", allowsDecimal: false, allowsNegative: true), "-3")
    }

    // MARK: ↑↓ の増減

    func testStepDefaultsToOne() {
        XCTAssertEqual(MacNumberInput.stepped("5", direction: 1, min: nil, max: nil, step: nil, allowsDecimal: false), "6")
        XCTAssertEqual(MacNumberInput.stepped("5", direction: -1, min: nil, max: nil, step: nil, allowsDecimal: false), "4")
        XCTAssertEqual(MacNumberInput.stepped("", direction: 1, min: nil, max: nil, step: nil, allowsDecimal: false), "1")
    }

    func testStepClampsToMinMax() {
        XCTAssertEqual(MacNumberInput.stepped("10", direction: 1, min: 0, max: 10, step: 1, allowsDecimal: false), "10")
        XCTAssertEqual(MacNumberInput.stepped("0", direction: -1, min: 0, max: 10, step: 1, allowsDecimal: false), "0")
        XCTAssertEqual(MacNumberInput.stepped("", direction: 1, min: 5, max: 10, step: 1, allowsDecimal: false), "5")
        XCTAssertEqual(MacNumberInput.stepped("99", direction: -1, min: 0, max: 10, step: 1, allowsDecimal: false), "10")
    }

    func testStepRespectsStepAndRoundsDecimals() {
        XCTAssertEqual(MacNumberInput.stepped("0.2", direction: 1, min: nil, max: nil, step: 0.1, allowsDecimal: true), "0.3")
        XCTAssertEqual(MacNumberInput.stepped("1", direction: 1, min: nil, max: nil, step: 0.25, allowsDecimal: true), "1.25")
        XCTAssertEqual(MacNumberInput.stepped("100", direction: 1, min: nil, max: nil, step: 50, allowsDecimal: false), "150")
    }

    func testStepIgnoresBadDSL() {
        // step ≤ 0 は 1、max < min は max を無視
        XCTAssertEqual(MacNumberInput.stepped("1", direction: 1, min: nil, max: nil, step: -3, allowsDecimal: false), "2")
        XCTAssertEqual(MacNumberInput.stepped("5", direction: 1, min: 3, max: 1, step: 1, allowsDecimal: false), "6")
    }

    // MARK: picker の形

    func testPickerStyleByOptionCount() {
        XCTAssertEqual(MacPickerStyle.choose(hint: nil, optionCount: 3), .radioGroup)
        XCTAssertEqual(MacPickerStyle.choose(hint: nil, optionCount: 4), .radioGroup)
        XCTAssertEqual(MacPickerStyle.choose(hint: nil, optionCount: 5), .menu)
    }

    func testPickerStyleMirrorsDSLHint() {
        XCTAssertEqual(MacPickerStyle.choose(hint: "menu", optionCount: 2), .menu)
        XCTAssertEqual(MacPickerStyle.choose(hint: "segmented", optionCount: 9), .segmented)
        XCTAssertEqual(MacPickerStyle.choose(hint: "wheel", optionCount: 3), .radioGroup)
        XCTAssertEqual(MacPickerStyle.choose(hint: "wheel", optionCount: 12), .menu)
    }

    func testDatePickerStyle() {
        XCTAssertEqual(MacDatePickerStyle.choose(includesDate: true), .fieldWithCalendar)
        XCTAssertEqual(MacDatePickerStyle.choose(includesDate: false), .fieldWithStepper)
    }

    // MARK: 日本語変換中の Return

    @MainActor
    private final class FakeEditor: MarkedTextProviding {
        var marked: Bool
        init(marked: Bool) { self.marked = marked }
        func hasMarkedText() -> Bool { marked }
    }

    @MainActor
    func testReturnBlockedWhileComposing() {
        XCTAssertFalse(IMEReturnGuard.allowsReturn(FakeEditor(marked: true)))
        XCTAssertTrue(IMEReturnGuard.allowsReturn(FakeEditor(marked: false)))
        XCTAssertTrue(IMEReturnGuard.allowsReturn(nil))
    }

    @MainActor
    func testRealTextViewReportsMarkedText() {
        let view = NSTextView()
        XCTAssertTrue(IMEReturnGuard.allowsReturn(view))
        view.setMarkedText("へんかん", selectedRange: NSRange(location: 4, length: 0), replacementRange: NSRange(location: NSNotFound, length: 0))
        XCTAssertFalse(IMEReturnGuard.allowsReturn(view))
        view.unmarkText()
        XCTAssertTrue(IMEReturnGuard.allowsReturn(view))
    }
}
