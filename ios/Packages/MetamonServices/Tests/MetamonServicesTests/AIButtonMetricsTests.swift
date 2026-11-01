#if !APP_STORE_REVIEW
import XCTest
import MetamonDSL
@testable import MetamonServices

/// #446: aiButton の端末内 AI の集計。日ごとに貯め、前の日までを 1 回で送る。ラベルと回数だけ。
@MainActor
final class AIButtonMetricsTests: XCTestCase {
    private var defaults: UserDefaults!
    private var calendar: Calendar = {
        var c = Calendar(identifier: .gregorian)
        c.timeZone = TimeZone(identifier: "Asia/Tokyo")!
        return c
    }()

    override func setUp() {
        defaults = UserDefaults(suiteName: "AIButtonMetricsTests.\(UUID().uuidString)")
    }

    private func date(_ day: Int, hour: Int = 12) -> Date {
        calendar.date(from: DateComponents(year: 2026, month: 11, day: day, hour: hour))!
    }

    private func recorder() -> AIButtonMetricsRecorder { AIButtonMetricsRecorder(defaults: defaults, calendar: calendar) }

    private final class Sender: AIButtonMetricsSender, @unchecked Sendable {
        var sent: [AIButtonMetricsReport] = []
        var error: (any Error)?
        func send(_ report: AIButtonMetricsReport) async throws {
            if let error { throw error }
            sent.append(report)
        }
    }

    func testAggregatesPerDayAndSendsOnlyPastDays() async throws {
        let r = recorder()
        r.record(.shown(.modelNotReady), now: date(1))
        r.record(.shown(.available), now: date(1, hour: 13))
        r.record(.ran(.summarize, failure: nil), now: date(1))
        r.record(.ran(.summarize, failure: .contextTooLong), now: date(1))
        r.record(.ran(.classify, failure: .invalidResponse), now: date(1))
        r.record(.shown(.deviceNotEligible), now: date(2))  // 今日の分は送らない

        let s = Sender()
        await r.flush(sender: s, now: date(2))
        XCTAssertEqual(s.sent.count, 1)
        let day = try XCTUnwrap(s.sent.first?.days.first)
        XCTAssertEqual(s.sent.first?.days.count, 1)
        XCTAssertEqual(day.day, "2026-11-01")
        XCTAssertEqual(day.availability, .available)  // その日の最後の値
        XCTAssertEqual(day.tasks.map(\.task), [.classify, .summarize])
        XCTAssertEqual(day.tasks.last?.attempts, 2)
        XCTAssertEqual(day.tasks.last?.failures, ["context_too_long": 1])

        // 送った日は消える。次の日に前日分だけ送る
        await r.flush(sender: s, now: date(2, hour: 18))
        XCTAssertEqual(s.sent.count, 1)
        await r.flush(sender: s, now: date(3))
        XCTAssertEqual(s.sent.last?.days.map(\.day), ["2026-11-02"])
        XCTAssertEqual(s.sent.last?.days.first?.availability, .deviceNotEligible)
        XCTAssertEqual(s.sent.last?.days.first?.tasks, [])
    }

    func testBodyHasOnlyLabelsAndCounts() throws {
        let r = recorder()
        r.record(.ran(.rewrite, failure: .guardrail), now: date(1))
        let report = try XCTUnwrap(r.pendingReport(now: date(2)))
        let json = String(decoding: try JSONEncoder().encode(report), as: UTF8.self)
        let obj = try JSONSerialization.jsonObject(with: Data(json.utf8)) as? [String: Any]
        XCTAssertEqual(Set((obj ?? [:]).keys), ["days"])
        let day = (obj?["days"] as? [[String: Any]])?.first
        XCTAssertEqual(Set((day ?? [:]).keys), ["reportId", "day", "availability", "tasks"])
        XCTAssertTrue(json.contains(#""availability":"available""#))  // 端末内で動いた日は available
        XCTAssertTrue(json.contains(#""guardrail":1"#))
    }

    func testNetworkFailureKeepsAndBadRequestDrops() async {
        let r = recorder()
        r.record(.shown(.appleIntelligenceOff), now: date(1))
        let s = Sender()
        s.error = URLError(.notConnectedToInternet)
        await r.flush(sender: s, now: date(2))
        XCTAssertNotNil(r.pendingReport(now: date(2)))
        s.error = APIError.server(status: 400, code: "bad_request", message: "x")
        await r.flush(sender: s, now: date(2))
        XCTAssertNil(r.pendingReport(now: date(2)))
    }

    func testDisableClearsAndStopsCounting() async {
        let r = recorder()
        r.record(.shown(.available), now: date(1))
        r.setEnabled(false)
        r.record(.ran(.suggest, failure: .refusal), now: date(1))
        XCTAssertNil(r.pendingReport(now: date(2)))
        // 保存される
        XCTAssertFalse(recorder().isEnabled)
    }

    func testPersistsAndPrunesOldDays() async {
        let r = recorder()
        r.record(.shown(.osTooOld), now: date(1))
        XCTAssertEqual(recorder().pendingReport(now: date(2))?.days.first?.availability, .osTooOld)
        // keepDays を過ぎた日は捨てる
        let later = recorder()
        let s = Sender()
        await later.flush(sender: s, now: date(20))
        XCTAssertTrue(s.sent.isEmpty)
        XCTAssertNil(later.pendingReport(now: date(20)))
    }

    func testUnknownTaskBecomesOther() {
        XCTAssertEqual(AIButtonMetric.Task(dslTask: "要約して"), .other)
        XCTAssertEqual(AIButtonMetric.Task(dslTask: nil), .other)
        XCTAssertEqual(AIButtonMetric.Task(dslTask: "generate"), .generate)
    }
}
#endif
