import XCTest
import SwiftUI
import Observation
import MetamonDSL
@testable import MetamonRenderer

/// AI が作る DSL の極端な値(min > max・step ≤ 0・巨大な数)。
final class ExtremeDSLValueTests: XCTestCase {
    func testRangeFixesInvertedAndEmptyBounds() {
        XCTAssertEqual(NumericInput.range(min: nil, max: nil, defaultMin: 0, defaultMax: 100), 0...100)
        let inverted = NumericInput.range(min: 10, max: 5, defaultMin: 0, defaultMax: 100)
        XCTAssertEqual(inverted.lowerBound, 10)
        XCTAssertGreaterThan(inverted.upperBound, 10, "min > max でも幅 0 にしない")
        let huge = NumericInput.range(min: 1e20, max: 0, defaultMin: 0, defaultMax: 100)
        XCTAssertGreaterThan(huge.upperBound, huge.lowerBound, "1e20 + 0.001 は 1e20 のままなので nextUp を使う")
    }

    func testStepRejectsZeroAndNegative() {
        XCTAssertNil(NumericInput.step(0))
        XCTAssertNil(NumericInput.step(-1))
        XCTAssertNil(NumericInput.step(nil))
        XCTAssertEqual(NumericInput.step(0.5), 0.5)
    }

    func testRatingSurvivesHugeValues() {
        XCTAssertEqual(NumericInput.ratingCount(nil), 5)
        XCTAssertEqual(NumericInput.ratingCount(1e300), 10)
        XCTAssertEqual(NumericInput.ratingCount(-3), 1)
        XCTAssertEqual(NumericInput.ratingValue(1e300, count: 5), 5)
        XCTAssertEqual(NumericInput.ratingValue(-1e300, count: 5), 0)
        XCTAssertEqual(NumericInput.ratingValue(.nan, count: 5), 0)
        XCTAssertEqual(NumericInput.ratingValue(3.4, count: 5), 3)
    }

    func testClampedIntDoesNotTrap() {
        XCTAssertEqual(NumericInput.clampedInt(1e300), Int.max)
        XCTAssertEqual(NumericInput.clampedInt(-.infinity), Int.min)
        XCTAssertEqual(NumericInput.clampedInt(.nan), 0)
        XCTAssertEqual(NumericInput.clampedInt(42.6), 43)
        XCTAssertEqual(HealthStatLeaf.stepsText(1e30).isEmpty, false)
    }

    func testEditorLinesClampHugeMinLines() {
        XCTAssertEqual(TextInputRules.editorLines(minLines: nil), 4...10)
        XCTAssertEqual(TextInputRules.editorLines(minLines: Int.max).lowerBound, TextInputRules.maxEditorMinLines)
        XCTAssertEqual(TextInputRules.editorLines(minLines: -5).lowerBound, 1)
    }
}
