import XCTest
import MetamonDSL
@testable import Metamon

/// #486: systemLarge 用の「最近の記録」「直近 7 日」の組み立て。
@MainActor
final class WidgetSnapshotWriterTests: XCTestCase {
    private var calendar: Calendar {
        // date field("yyyy-MM-dd")は端末のタイムゾーンで読むので、暦も端末に合わせる
        Calendar(identifier: .gregorian)
    }
    /// 2026-10-31 12:00(端末ローカル)
    private var now: Date { calendar.date(from: DateComponents(year: 2026, month: 10, day: 31, hour: 12))! }

    private func dsl(filter: String = #","filter":{"field":"date","op":"thisMonth"}"#, aggregate: String = "sum") throws -> AppDSL {
        let json = """
        {"schemaVersion":1,"app":{"name":"家計簿"},"initialScreen":"home",
         "entities":[{"id":"expense","name":"支出","titleField":"memo","fields":[
           {"id":"amount","name":"金額","type":"currency","unit":"円"},
           {"id":"memo","name":"メモ","type":"string"},
           {"id":"secret","name":"正解","type":"string","hidden":true},
           {"id":"date","name":"日付","type":"date"}]}],
         "screens":[{"id":"home","title":"家計簿","components":[
           {"type":"stat","label":"今月の支出","entity":"expense","aggregate":"\(aggregate)","field":"amount","format":"currency"\(filter)}]}]}
        """
        return try JSONDecoder().decode(AppDSL.self, from: Data(json.utf8))
    }

    private func record(_ id: String, _ amount: Double, _ memo: String, date: String, created: String) -> Record {
        Record(id: id, data: ["amount": .number(amount), "memo": .string(memo), "secret": .string("x"), "date": .string(date)],
               createdAt: created)
    }

    private var records: [String: [Record]] {
        ["expense": [
            record("a", 500, "コンビニ", date: "2026-10-31", created: "2026-10-31T01:00:00Z"),
            record("b", 300, "電車", date: "2026-10-31", created: "2026-10-31T00:00:00Z"),
            record("c", 1000, "ランチ", date: "2026-10-29", created: "2026-10-29T03:00:00Z"),
            record("d", 9999, "先月", date: "2026-09-01", created: "2026-09-01T03:00:00Z"),
        ]]
    }

    func testRecentRowsNewestFirstWithoutHiddenFields() throws {
        let s = WidgetSnapshotWriter.makeSnapshot(dsl: try dsl(), records: records, previous: nil, now: now, calendar: calendar)
        XCTAssertEqual(s.recentTitle, "支出")
        let recent = try XCTUnwrap(s.recent)
        XCTAssertEqual(recent.map(\.id), ["a", "b", "c", "d"])
        XCTAssertEqual(recent[0].title, "コンビニ")
        XCTAssertEqual(recent[0].value, "¥500")
        XCTAssertNotNil(recent[0].detail)
        XCTAssertFalse(recent.contains { $0.title == "x" || $0.value == "x" || $0.detail == "x" }, "hidden は出さない")
    }

    func testRecentCappedAtMax() throws {
        let many = (0..<9).map { record("r\($0)", 1, "m\($0)", date: "2026-10-31", created: "2026-10-31T0\($0):00:00Z") }
        let s = WidgetSnapshotWriter.makeSnapshot(dsl: try dsl(), records: ["expense": many], previous: nil, now: now, calendar: calendar)
        XCTAssertEqual(s.recent?.count, WidgetSnapshotWriter.maxRecent)
        XCTAssertEqual(s.recent?.first?.id, "r8")
    }

    func testTrendSumsPerDayAndDropsPeriodFilter() throws {
        let s = WidgetSnapshotWriter.makeSnapshot(dsl: try dsl(), records: records, previous: nil, now: now, calendar: calendar)
        let trend = try XCTUnwrap(s.trend)
        XCTAssertEqual(trend.points.count, 7)
        XCTAssertEqual(trend.points.last?.day, calendar.startOfDay(for: now))
        XCTAssertEqual(trend.points.map(\.value), [0, 0, 0, 0, 1000, 0, 800], "今月 filter は外し、7 日より前は入れない")
        XCTAssertEqual(trend.total, "¥1,800")
        XCTAssertEqual(trend.metric, "金額")
    }

    func testTrendKeepsNonPeriodFilterAndCount() throws {
        let d = try dsl(filter: #","filter":{"field":"amount","op":"gte","value":400}"#, aggregate: "count")
        let trend = try XCTUnwrap(WidgetSnapshotWriter.makeSnapshot(dsl: d, records: records, previous: nil, now: now, calendar: calendar).trend)
        XCTAssertEqual(trend.points.map(\.value), [0, 0, 0, 0, 1, 0, 1])
        XCTAssertNil(trend.metric, "件数は metric 無し")
    }

    func testPickAggregateHasNoTrend() throws {
        let s = WidgetSnapshotWriter.makeSnapshot(dsl: try dsl(aggregate: "latest"), records: records, previous: nil, now: now, calendar: calendar)
        XCTAssertNil(s.trend)
        XCTAssertNotNil(s.recent)
    }

    func testNotLoadedKeepsPreviousLargeData() throws {
        let first = WidgetSnapshotWriter.makeSnapshot(dsl: try dsl(), records: records, previous: nil, now: now, calendar: calendar)
        let reloaded = WidgetSnapshotWriter.makeSnapshot(dsl: try dsl(), records: [:], previous: first, now: now, calendar: calendar)
        XCTAssertEqual(reloaded.recent, first.recent)
        XCTAssertEqual(reloaded.trend, first.trend)
        XCTAssertEqual(WidgetSnapshotWriter.makeSnapshot(dsl: try dsl(), records: ["expense": []], previous: first, now: now, calendar: calendar).recent, [],
                       "取得済みで 0 件なら空")
    }

    func testSameContentIgnoresUpdatedAt() throws {
        let a = WidgetSnapshotWriter.makeSnapshot(dsl: try dsl(), records: records, previous: nil, now: now, calendar: calendar)
        var b = a
        b.updatedAt = now.addingTimeInterval(60)
        XCTAssertTrue(b.hasSameContent(as: a))
        b.recent?.removeLast()
        XCTAssertFalse(b.hasSameContent(as: a))
        XCTAssertFalse(a.hasSameContent(as: nil))
    }

    func testOldSnapshotJSONStillDecodes() throws {
        let old = #"{"appName":"家計簿","stats":[{"key":"k","label":"L","value":"1"}],"updatedAt":0}"#
        let s = try JSONDecoder().decode(WidgetSnapshot.self, from: Data(old.utf8))
        XCTAssertNil(s.recent)
        XCTAssertNil(s.trend)
    }
}
