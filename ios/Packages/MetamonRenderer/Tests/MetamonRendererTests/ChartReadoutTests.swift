import XCTest
@testable import MetamonRenderer

/// UX 点検 L4: グラフの値の読み取りと VoiceOver の要約。
final class ChartReadoutTests: XCTestCase {
    private let ja = Locale(identifier: "ja_JP")

    func testFormatKeepsIntegersAndOneDecimal() {
        XCTAssertEqual(ChartReadout.format(12, locale: ja), "12")
        XCTAssertEqual(ChartReadout.format(1234, locale: ja), "1,234")
        XCTAssertEqual(ChartReadout.format(2.46, locale: ja), "2.5")
    }

    func testLineShowsLabelAndValue() {
        XCTAssertEqual(ChartReadout.line(label: "3月", value: 12, locale: ja), "3月: 12")
    }

    func testPieSelectionMapsCumulativeValueToSlice() {
        let values: [Double] = [2, 3, 5]
        XCTAssertEqual(ChartReadout.index(atCumulative: 0.5, values: values), 0)
        XCTAssertEqual(ChartReadout.index(atCumulative: 2, values: values), 0)
        XCTAssertEqual(ChartReadout.index(atCumulative: 4.9, values: values), 1)
        XCTAssertEqual(ChartReadout.index(atCumulative: 9.5, values: values), 2)
        XCTAssertEqual(ChartReadout.index(atCumulative: 99, values: values), 2, "端を越えても最後の扇")
        XCTAssertNil(ChartReadout.index(atCumulative: 1, values: []))
    }

    func testSummaryNamesHighestAndLowest() {
        let s = ChartReadout.summary(labels: ["1月", "2月", "3月"], values: [2, 5, 12], locale: ja)
        XCTAssertTrue(s.contains("3"), s)
        XCTAssertTrue(s.contains("3月") && s.contains("12"), "最大を言う: \(s)")
        XCTAssertTrue(s.contains("1月") && s.contains("2"), "最小を言う: \(s)")
    }

    func testSummaryWithOneItemAndEmpty() {
        let one = ChartReadout.summary(labels: ["食費"], values: [800], locale: ja)
        XCTAssertTrue(one.contains("食費") && one.contains("800"), one)
        XCTAssertFalse(ChartReadout.summary(labels: [], values: [], locale: ja).isEmpty)
    }
}
