import XCTest
import MetamonDSL
import MetamonRenderer
@testable import Metamon

/// #423: DSL → ライブアクティビティの中身(ActivityKit は実機 / シミュレータの手元確認。ここでは組み立てだけ)。
@MainActor
final class LiveActivityContentTests: XCTestCase {
    private let t0 = Date(timeIntervalSince1970: 1_000_000)
    private typealias Clock = TailorActivityAttributes.Clock

    // MARK: timerDisplay.liveActivity

    func testCountdownRunningShowsUntilEnd() {
        let plan = LiveActivityContent.plan(for: TimerActivityUpdate(key: "k", label: "カップ麺", total: 180, run: TimerRun(base: 0, startedAt: t0)))
        guard case .show(let state, let stale) = plan else { return XCTFail("\(plan)") }
        XCTAssertEqual(state.title, "カップ麺")
        XCTAssertEqual(state.clock, .countdown(start: t0, end: t0 + 180))
        XCTAssertEqual(stale, t0 + 180, "終わったらロック画面が「終了」に切り替わる")
        XCTAssertFalse(state.finished)
    }

    func testStopwatchAndPause() {
        let running = LiveActivityContent.plan(for: TimerActivityUpdate(key: "k", label: "プランク", total: nil, run: TimerRun(base: 20, startedAt: t0)))
        guard case .show(let s1, nil) = running else { return XCTFail("\(running)") }
        XCTAssertEqual(s1.clock, .elapsed(since: t0 - 20))

        let paused = LiveActivityContent.plan(for: TimerActivityUpdate(key: "k", label: "カップ麺", total: 180, run: TimerRun(base: 60)))
        guard case .show(let s2, nil) = paused else { return XCTFail("\(paused)") }
        XCTAssertEqual(s2.clock, .paused(seconds: 120))
        XCTAssertNotNil(s2.subtitle)
    }

    func testFinishLingersAndResetEnds() {
        let done = LiveActivityContent.plan(for: TimerActivityUpdate(key: "k", label: "麺", total: 180, run: TimerRun(base: 180)))
        guard case .finish(let state) = done else { return XCTFail("\(done)") }
        XCTAssertTrue(state.finished)
        XCTAssertEqual(state.clock, .paused(seconds: 0))
        XCTAssertEqual(LiveActivityContent.plan(for: TimerActivityUpdate(key: "k", label: "麺", total: 180, run: TimerRun())), .end)
    }

    func testKeysSeparateTimerAndAction() {
        XCTAssertNotEqual(LiveActivityContent.timerKey("a"), LiveActivityContent.appKey("a"))
    }

    // MARK: startActivity

    func testActionWithMinutesCountsDown() {
        let (state, stale) = LiveActivityContent.state(for: ActivitySpec(title: "行列に並び中", subtitle: " ", minutes: 30), stat: nil, now: t0)
        XCTAssertEqual(state.clock, .countdown(start: t0, end: t0 + 1800))
        XCTAssertEqual(stale, t0 + 1800)
        XCTAssertNil(state.subtitle, "空白だけの subtitle は出さない")
    }

    func testActionWithoutMinutesCountsUpAndCapsAt8h() {
        let (up, upStale) = LiveActivityContent.state(for: ActivitySpec(title: "運転中"), stat: nil, now: t0)
        XCTAssertEqual(up.clock, .elapsed(since: t0))
        XCTAssertNil(upStale)
        let (capped, _) = LiveActivityContent.state(for: ActivitySpec(title: "x", minutes: 10_000), stat: nil, now: t0)
        XCTAssertEqual(capped.clock, .countdown(start: t0, end: t0 + 480 * 60))
    }

    // MARK: stat / progressBar

    func testStatReadingFromExample() throws {
        let dsl = try DSLExamples.load("ramen_timer", bundle: Bundle(for: AppRuntime.self))
        let now = Date()
        let records: [String: [Record]] = ["bowl": [
            Record(id: "1", data: ["name": .string("a"), "date": .string(ISO8601DateFormatter().string(from: now))]),
            Record(id: "2", data: ["name": .string("b"), "date": .string(ISO8601DateFormatter().string(from: now))]),
        ]]
        let reading = try XCTUnwrap(LiveActivityContent.statReading(id: "month", dsl: dsl, records: records))
        XCTAssertEqual(reading.label, "今月の杯数")
        XCTAssertNotNil(reading.value)
        XCTAssertTrue(reading.value?.contains("2") == true, "\(reading)")
        XCTAssertNil(LiveActivityContent.statReading(id: "month", dsl: dsl, records: [:])?.value, "未取得は数字なし")
        XCTAssertNil(LiveActivityContent.statReading(id: "timer", dsl: dsl, records: records), "stat / progressBar 以外は読まない")
        XCTAssertNil(LiveActivityContent.statReading(id: "nope", dsl: dsl, records: records))
    }

    func testProgressBarReading() throws {
        let json = #"""
        {"schemaVersion":1,"app":{"name":"貯金"},"initialScreen":"home",
         "entities":[{"id":"save","name":"貯金","fields":[{"id":"amount","name":"額","type":"integer"}]}],
         "screens":[{"id":"home","title":"貯金","components":[
           {"type":"progressBar","id":"goal","label":"目標","entity":"save","aggregate":"sum","field":"amount","target":1000}]}]}
        """#
        let dsl = try JSONDecoder().decode(AppDSL.self, from: Data(json.utf8))
        let records: [String: [Record]] = ["save": [Record(id: "1", data: ["amount": .number(250)])]]
        let reading = try XCTUnwrap(LiveActivityContent.statReading(id: "goal", dsl: dsl, records: records))
        XCTAssertEqual(reading.progress ?? -1, 0.25, accuracy: 0.0001)
        XCTAssertEqual(reading.value, "25%")
    }

    func testRefreshOnlyWhenChanged() {
        let (state, _) = LiveActivityContent.state(for: ActivitySpec(title: "並び中", stat: "month"),
                                                   stat: .init(label: "杯数", value: "1 件"), now: t0)
        XCTAssertNil(LiveActivityContent.refreshed(state, with: .init(label: "杯数", value: "1 件")))
        let next = LiveActivityContent.refreshed(state, with: .init(label: "杯数", value: "2 件"))
        XCTAssertEqual(next?.value, "2 件")
        XCTAssertEqual(next?.clock, state.clock, "時間は変えない")
    }
}
