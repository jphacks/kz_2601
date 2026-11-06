import XCTest
import SwiftUI
import Observation
import MetamonDSL
@testable import MetamonRenderer

/// 取得失敗(`failed`)を持てるランタイム。
@MainActor
@Observable
private final class OfflineRuntime: DSLRuntime {
    var dsl: AppDSL
    var paths: [String: [Route]] = [:]
    var selectedTab = ""
    var loading: Set<String> = []
    var failed: Set<String> = []
    var store: [String: [Record]] = [:]

    init(dsl: AppDSL) { self.dsl = dsl }

    func records(for entity: String) -> [Record] { store[entity] ?? [] }
    func load(entities: Set<String>, force: Bool) async {}
    func run(_ action: Action, _ ctx: ActionContext) async {}
    func close(_ route: Route) {}
    func pathBinding(_ stack: String) -> Binding<[Route]> { .constant([]) }
}

/// AI が作る DSL の極端な値(min > max・step ≤ 0・巨大な数)と、取得失敗の表示。
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

    /// 取得に失敗して記録が無い時、progressBar / gauge は「0%」「0」ではなく「—」(#85・stat と同じ)。
    @MainActor func testProgressBarUnavailableWhenFetchFailed() throws {
        var url = URL(fileURLWithPath: #filePath)
        for _ in 0..<6 { url.deleteLastPathComponent() }
        let dsl = try AppDSL.load(contentsOf: url.appending(path: "docs/dsl/examples/vocab.json"))
        let rt = OfflineRuntime(dsl: dsl)
        let c = try JSONDecoder().decode(Component.self, from: Data(#"{"type":"progressBar","entity":"word","aggregate":"count","target":10}"#.utf8))
        XCTAssertFalse(ProgressBarLeaf.isUnavailable(c, rt: rt), "取得に失敗していなければ 0% のまま")
        rt.failed = ["word"]
        XCTAssertTrue(ProgressBarLeaf.isUnavailable(c, rt: rt))
        rt.store["word"] = [Record(id: "w1", data: ["term": "apple"])]
        XCTAssertFalse(ProgressBarLeaf.isUnavailable(c, rt: rt), "手元に記録があれば集計する")
        let literal = try JSONDecoder().decode(Component.self, from: Data(#"{"type":"progressBar","value":0.4}"#.utf8))
        rt.store = [:]
        XCTAssertFalse(ProgressBarLeaf.isUnavailable(literal, rt: rt), "value リテラルは取得に関係しない")
    }
}
