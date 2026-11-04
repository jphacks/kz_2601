import XCTest
@testable import MetamonDSL

/// 記録の保存期間(#482 `Entity.retentionDays`): 期限切れの判定・共有の扱い・確認状態。
final class RecordRetentionTests: XCTestCase {
    private func entity(_ extra: String = "") throws -> Entity {
        let json = #"{"id":"diary","name":"日記","fields":[{"id":"body","name":"本文","type":"text"}]"# + extra + "}"
        return try JSONDecoder().decode(Entity.self, from: Data(json.utf8))
    }

    private let now = ISO8601DateFormatter().date(from: "2026-10-31T12:00:00Z")!

    private func record(_ id: String, daysAgo: Double?, author: String? = nil) -> Record {
        let created = daysAgo.map { ISO8601DateFormatter().string(from: now.addingTimeInterval(-$0 * 86_400)) }
        return Record(id: id, data: [:], createdAt: created, author: author)
    }

    func testDecodesRetentionDaysLeniently() throws {
        XCTAssertEqual(try entity(#","retentionDays":7"#).retentionDays, 7)
        XCTAssertEqual(try entity(#","retentionDays":30.0"#).retentionDays, 30)
        XCTAssertNil(try entity().retentionDays)
        // 壊れた値は entity ごと落とさず「消さない」
        XCTAssertNil(try entity(#","retentionDays":"7""#).retentionDays)
        XCTAssertNil(try entity(#","retentionDays":0"#).retentionDays)
        XCTAssertNil(try entity(#","retentionDays":-3"#).retentionDays)
        XCTAssertNil(try entity(#","retentionDays":1.5"#).retentionDays)
    }

    func testExpiredUsesCreatedAt() throws {
        let e = try entity(#","retentionDays":7"#)
        let rows = [record("new", daysAgo: 1), record("edge", daysAgo: 7), record("old", daysAgo: 7.01),
                    record("older", daysAgo: 40), record("nodate", daysAgo: nil),
                    Record(id: "bad", data: [:], createdAt: "yesterday")]
        let gone = RecordRetention.expired(rows, in: e, days: 7, now: now, uid: "me").map(\.id)
        XCTAssertEqual(gone, ["old", "older"]) // ちょうど 7 日は残す・日時の無い / 読めないものは消さない
        // 小数秒つきの RFC3339(appconfig の形)
        let frac = Record(id: "frac", data: [:], createdAt: "2026-10-01T00:00:00.123Z")
        XCTAssertEqual(RecordRetention.expired([frac], in: e, days: 7, now: now, uid: nil).map(\.id), ["frac"])
    }

    func testSharedEntityDeletesOnlyOwnRecords() throws {
        let e = try entity(#","scope":"public","retentionDays":7"#)
        let rows = [record("mine", daysAgo: 10, author: "me"), record("theirs", daysAgo: 10, author: "other"),
                    record("legacy", daysAgo: 10)]
        XCTAssertEqual(RecordRetention.expired(rows, in: e, days: 7, now: now, uid: "me").map(\.id), ["mine"])
        XCTAssertTrue(RecordRetention.expired(rows, in: e, days: 7, now: now, uid: nil).isEmpty) // uid 不明は消さない
        // private は author に関係なく全部自分のもの
        let p = try entity(#","retentionDays":7"#)
        XCTAssertEqual(RecordRetention.expired(rows, in: p, days: 7, now: now, uid: nil).count, 3)
    }

    func testConsentGatesShorterPeriods() throws {
        let e = try entity(#","retentionDays":7"#)
        XCTAssertNil(RecordRetention.activeDays(e, consented: nil))  // 未確認は消さない
        XCTAssertEqual(RecordRetention.activeDays(e, consented: 7), 7)
        XCTAssertEqual(RecordRetention.activeDays(e, consented: 3), 7) // 延ばしただけなら確認し直さない
        XCTAssertNil(RecordRetention.activeDays(e, consented: 30))    // 短くしたら確認し直す
        XCTAssertNil(RecordRetention.activeDays(try entity(), consented: 7))

        let dsl = try JSONDecoder().decode(AppDSL.self, from: Data(#"""
        {"schemaVersion":1,"app":{"name":"日記","voiceEntry":"none"},"initialScreen":"home","screens":[{"id":"home","title":"日記","components":[]}],
         "entities":[{"id":"diary","name":"日記","retentionDays":7,"fields":[]},{"id":"tags","name":"タグ","fields":[]},
                     {"id":"logs","name":"ログ","retentionDays":30,"fields":[]}]}
        """#.utf8))
        XCTAssertEqual(RecordRetention.pending(dsl, consented: [:]).map(\.id), ["diary", "logs"])
        XCTAssertEqual(RecordRetention.pending(dsl, consented: ["diary": 7, "logs": 60]).map(\.id), ["logs"])
        XCTAssertTrue(RecordRetention.pending(dsl, consented: ["diary": 7, "logs": 30]).isEmpty)
    }

    func testPeriodLabel() {
        XCTAssertEqual(RecordRetention.periodLabel(days: 7), "1週間")
        XCTAssertEqual(RecordRetention.periodLabel(days: 14), "2週間")
        XCTAssertEqual(RecordRetention.periodLabel(days: 30), "30日")
        XCTAssertEqual(RecordRetention.periodLabel(days: 365), "1年")
        XCTAssertEqual(RecordRetention.periodLabel(days: 1), "1日")
        XCTAssertEqual(RecordRetention.periodLabel(days: 7, english: true), "1 week")
        XCTAssertEqual(RecordRetention.periodLabel(days: 90, english: true), "90 days")
    }
}
