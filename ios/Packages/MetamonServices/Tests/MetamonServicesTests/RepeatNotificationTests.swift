import XCTest
import UserNotifications
import MetamonDSL
@testable import MetamonServices

/// #266: レコード基準の通知を毎月・毎年くり返す(サブスクの更新日・誕生日)。
final class RepeatNotificationTests: XCTestCase {
    private let cal: Calendar = {
        var c = Calendar(identifier: .gregorian)
        c.timeZone = TimeZone(identifier: "Asia/Tokyo")!
        return c
    }()
    private var savedTZ: TimeZone?

    override func setUp() {
        savedTZ = FieldCoding.dateFormatter.timeZone
        FieldCoding.dateFormatter.timeZone = cal.timeZone // "YYYY-MM-DD" を JST の 0:00 として読む
    }

    override func tearDown() {
        FieldCoding.dateFormatter.timeZone = savedTZ
    }

    private func date(_ y: Int, _ mo: Int, _ d: Int, _ h: Int = 9, _ m: Int = 0) -> Date {
        cal.date(from: DateComponents(year: y, month: mo, day: d, hour: h, minute: m))!
    }

    private func plan(_ repeat: NotificationSpec.Repeat, _ value: String, offset: Int?, now: Date, time: String? = nil) -> [PlannedNotification] {
        let spec = NotificationSpec(id: "renew", title: "更新日", repeat: `repeat`, time: time, entity: "subs", field: "renew_on", offsetMinutes: offset, titleField: "name")
        let rec = Record(id: "r1", data: ["name": "動画サブスク", "renew_on": .string(value)])
        return NotificationPlanner.plan(appId: "a1", specs: [spec], records: ["subs": [rec]], now: now, calendar: cal)
    }

    private func fires(_ p: [PlannedNotification]) -> [Date] {
        p.compactMap { if case .at(let d) = $0.trigger { d } else { nil } }
    }

    /// 完了の条件: 更新日 10/30・1 日前 → 10/29 と 11/29 に予約される。
    func testSubscriptionRenewalDayBefore() {
        let p = plan(.monthly, "2026-10-30", offset: -1440, now: date(2026, 10, 1, 12))
        XCTAssertEqual(fires(p), [date(2026, 10, 29), date(2026, 11, 29), date(2026, 12, 29)])
        XCTAssertEqual(p.map(\.identifier), ["dsl.a1.renew.r1#0", "dsl.a1.renew.r1#1", "dsl.a1.renew.r1#2"])
        XCTAssertEqual(p[0].body, "動画サブスク")
        // 上限・近い順の選び方は従来どおり
        let chosen = NotificationPlanner.select(p, now: date(2026, 10, 1, 12), calendar: cal)
        XCTAssertEqual(chosen.map(\.1).prefix(2), [date(2026, 10, 29), date(2026, 11, 29)])
    }

    /// 更新日を過ぎても翌月以降が予約される(once だと何も残らなかった)。
    func testAfterRenewalDayStillSchedulesNextMonths() {
        let p = plan(.monthly, "2026-10-30", offset: -1440, now: date(2026, 10, 31, 12))
        XCTAssertEqual(fires(p), [date(2026, 11, 29), date(2026, 12, 29), date(2027, 1, 29)])
        XCTAssertTrue(plan(.once, "2026-10-30", offset: -1440, now: date(2026, 10, 31, 12)).isEmpty)
    }

    /// 2 月は月末(28 日)に寄せ、その 1 日前の 27 日。翌月は元の 30 日に戻る。
    func testMonthEndClampedInFebruary() {
        let p = plan(.monthly, "2026-10-30", offset: -1440, now: date(2027, 1, 30, 12))
        XCTAssertEqual(fires(p), [date(2027, 2, 27), date(2027, 3, 29), date(2027, 4, 29)])
        let p31 = plan(.monthly, "2026-01-31", offset: nil, now: date(2026, 1, 31, 10))
        XCTAssertEqual(fires(p31), [date(2026, 2, 28), date(2026, 3, 31), date(2026, 4, 30)])
    }

    /// 毎月同じ日(28 日以下)に鳴るなら、くり返しのトリガー 1 件(アプリを開かなくても鳴り続ける)。
    func testRepeatingMonthlyWhenDayIsStable() {
        let p = plan(.monthly, "2026-10-15", offset: -1440, now: date(2026, 11, 2, 12), time: "20:00")
        XCTAssertEqual(p.count, 1)
        XCTAssertEqual(p[0].identifier, "dsl.a1.renew.r1")
        XCTAssertEqual(p[0].trigger, .monthly(day: 14, hour: 20, minute: 0))
        XCTAssertEqual(p[0].nextFire(after: date(2026, 11, 2, 12), calendar: cal), date(2026, 11, 14, 20))
        let req = UserNotificationScheduler.request(p[0], calendar: cal)
        let t = req.trigger as? UNCalendarNotificationTrigger
        XCTAssertEqual(t?.repeats, true)
        XCTAssertEqual(t?.dateComponents.day, 14)
        XCTAssertNil(t?.dateComponents.month)
    }

    /// 1 日の 1 日前は「前の月の末日」で毎月変わる → 具体的な日付。
    func testFirstOfMonthDayBeforeIsConcrete() {
        let p = plan(.monthly, "2026-03-01", offset: -1440, now: date(2026, 11, 2, 12))
        XCTAssertEqual(fires(p), [date(2026, 11, 30), date(2026, 12, 31), date(2027, 1, 31)])
    }

    /// 誕生日: 毎年同じ月日ならくり返し 1 件。
    func testBirthdayYearlyRepeating() {
        let p = plan(.yearly, "1990-05-10", offset: nil, now: date(2026, 11, 2, 12))
        XCTAssertEqual(p.map(\.trigger), [.yearly(month: 5, day: 10, hour: 9, minute: 0)])
        XCTAssertEqual(p[0].nextFire(after: date(2026, 11, 2, 12), calendar: cal), date(2027, 5, 10))
        let t = UserNotificationScheduler.request(p[0], calendar: cal).trigger as? UNCalendarNotificationTrigger
        XCTAssertEqual(t?.repeats, true)
        XCTAssertEqual(t?.dateComponents.month, 5)
    }

    /// 2/29 生まれ: 平年は 2/28、閏年は 2/29 → 具体的な日付 2 件。
    func testLeapDayBirthday() {
        let p = plan(.yearly, "2004-02-29", offset: nil, now: date(2026, 11, 2, 12))
        XCTAssertEqual(fires(p), [date(2027, 2, 28), date(2028, 2, 29)])
    }

    /// 記念日の 1 週間前(3/1 の 7 日前は平年 2/22・閏年 2/23)。
    func testYearlyOffsetAcrossFebruaryIsConcrete() {
        let p = plan(.yearly, "2020-03-01", offset: -7 * 1440, now: date(2026, 11, 2, 12))
        XCTAssertEqual(fires(p), [date(2027, 2, 22), date(2028, 2, 23)])
    }

    /// 起点がまだ先なら、起点より前には鳴らさない。
    func testFutureAnchorStartsAtAnchor() {
        let p = plan(.monthly, "2027-03-15", offset: nil, now: date(2026, 11, 2, 12))
        XCTAssertEqual(fires(p), [date(2027, 3, 15), date(2027, 4, 15), date(2027, 5, 15)])
    }

    /// datetime の field は時刻もそのまま使う。
    func testDatetimeAnchorKeepsTime() {
        // オフセット付きの RFC 3339 を "HH:mm" と取り違えない
        XCTAssertNil(NotificationPlanner.parseTime("2026-01-31T23:00:00+09:00"))
        XCTAssertNotNil(NotificationPlanner.parseTime("7:05"))
        let p = plan(.monthly, "2026-01-31T23:00:00+09:00", offset: -60, now: date(2026, 11, 2, 12))
        XCTAssertEqual(fires(p), [date(2026, 11, 30, 22), date(2026, 12, 31, 22), date(2027, 1, 31, 22)])
    }

    /// 1 件のレコードが使う枠は monthlyAhead まで。全体は select が 60 件に収める。
    func testManyRecordsRespectLimit() {
        let spec = NotificationSpec(id: "renew", title: "t", repeat: .monthly, entity: "subs", field: "renew_on")
        let recs = (0..<40).map { Record(id: "r\($0)", data: ["renew_on": .string("2026-01-31")]) }
        let p = NotificationPlanner.plan(appId: "a1", specs: [spec], records: ["subs": recs], now: date(2026, 11, 2, 12), calendar: cal)
        XCTAssertEqual(p.count, 40 * NotificationPlanner.monthlyAhead)
        XCTAssertEqual(NotificationPlanner.select(p, now: date(2026, 11, 2, 12), calendar: cal).count, NotificationPlanner.limit)
    }

    /// 古い版は monthly / yearly を知らないので once として読む(落ちない)。新しい版は読める。
    func testDecodesMonthlyAndUnknownFallsBackToOnce() throws {
        let json = #"[{"id":"a","title":"t","body":"","repeat":"monthly"},{"id":"b","title":"t","body":"","repeat":"yearly"},{"id":"c","title":"t","body":"","repeat":"hourly"}]"#
        let specs = try JSONDecoder().decode([NotificationSpec].self, from: Data(json.utf8))
        XCTAssertEqual(specs.map(\.repeat), [.monthly, .yearly, .once])
    }

    /// 予約の保存形式(UserDefaults の JSON)を往復できる。
    func testTriggerCodableRoundTrip() throws {
        let p = [
            PlannedNotification(identifier: "x", title: "t", body: "", trigger: .monthly(day: 14, hour: 9, minute: 0)),
            PlannedNotification(identifier: "y", title: "t", body: "", trigger: .yearly(month: 5, day: 10, hour: 9, minute: 0)),
        ]
        XCTAssertEqual(try JSONDecoder().decode([PlannedNotification].self, from: JSONEncoder().encode(p)), p)
    }
}
