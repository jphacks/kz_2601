import XCTest
import MetamonDSL
@testable import MetamonServices

/// 止めるまで鳴るアラーム(`Notification.alarm`・AlarmKit)の組み立て・落とし方・後片付け。AlarmKit は `FakeAlarms` に差し替える。
final class AlarmPlannerTests: XCTestCase {
    private var cal: Calendar = {
        var c = Calendar(identifier: .gregorian)
        c.timeZone = TimeZone(identifier: "Asia/Tokyo")!
        return c
    }()
    /// 2026-10-25(日) 10:00 JST
    private var now: Date { cal.date(from: DateComponents(year: 2026, month: 10, day: 25, hour: 10, minute: 0))! }

    private func at(_ d: Int, _ h: Int, _ m: Int) -> Date {
        cal.date(from: DateComponents(year: 2026, month: 10, day: d, hour: h, minute: m))!
    }

    // MARK: - 時刻・繰り返しの組み立て

    func testFixedDailyWeeklyOnceToAlarmTriggers() {
        let specs = [
            NotificationSpec(id: "wake", title: "起床", repeat: .daily, time: "07:00", alarm: true),
            NotificationSpec(id: "gym", title: "ジム", repeat: .weekly, time: "06:30", weekday: "mon", alarm: true),
            NotificationSpec(id: "nap", title: "昼寝", repeat: .once, time: "13:30", alarm: true),
            NotificationSpec(id: "memo", title: "記録", repeat: .daily, time: "21:00"), // 普通の通知
        ]
        let p = NotificationPlanner.plan(appId: "a1", specs: specs, records: [:], now: now, calendar: cal)
        XCTAssertEqual(p.map(\.isAlarm), [true, true, true, false])
        XCTAssertNil(p[3].alarm, "普通の通知は alarm を持たない(保存の形が今まで通り)")
        let r = AlarmPlanner.requests(p, now: now, calendar: cal)
        XCTAssertEqual(r.map(\.identifier), ["dsl.a1.wake", "dsl.a1.gym", "dsl.a1.nap"])
        XCTAssertEqual(r[0].trigger, .relative(hour: 7, minute: 0, weekdays: [1, 2, 3, 4, 5, 6, 7]))
        XCTAssertEqual(r[1].trigger, .relative(hour: 6, minute: 30, weekdays: [2]))
        XCTAssertEqual(r[2].trigger, .fixed(at(25, 13, 30)))
    }

    func testRecordAlarmUsesRecordTimeAndLabel() {
        let spec = NotificationSpec(id: "alarm", title: "アラーム", repeat: .daily, entity: "alarm", field: "time",
                                    titleField: "label", enabledField: "on", alarm: true)
        let recs = ["alarm": [
            Record(id: "r1", data: ["time": "06:45", "label": "起きる", "on": .bool(true)]),
            Record(id: "r2", data: ["time": "08:00", "label": "休日", "on": .bool(false)]),
        ]]
        let r = AlarmPlanner.requests(NotificationPlanner.plan(appId: "a1", specs: [spec], records: recs, now: now, calendar: cal), now: now, calendar: cal)
        XCTAssertEqual(r.count, 1, "オフのレコードは鳴らさない")
        XCTAssertEqual(r[0].identifier, "dsl.a1.alarm.r1")
        XCTAssertEqual(r[0].trigger, .relative(hour: 6, minute: 45, weekdays: AlarmPlanner.everyDay))
        XCTAssertEqual(r[0].title, "アラーム 起きる")
    }

    func testButtonAlarmAndMonthlyNextOnly() {
        let button = NotificationPlanner.action(appId: "a1", spec: NotificationSpec(id: "n", title: "昼寝", repeat: .once, time: "13:00", alarm: true), now: now, calendar: cal)
        XCTAssertEqual(button?.isAlarm, true)
        let monthly = PlannedNotification(identifier: "dsl.a1.pay.r1", title: "支払日", body: "", trigger: .monthly(day: 27, hour: 9, minute: 0), alarm: true)
        XCTAssertEqual(AlarmPlanner.trigger(for: monthly, now: now, calendar: cal), .fixed(at(27, 9, 0)), "毎月は次の 1 回(開いた時に先へ延ばす)")
        let past = PlannedNotification(identifier: "x", title: "t", body: "", trigger: .at(at(25, 9, 0)), alarm: true)
        XCTAssertNil(AlarmPlanner.trigger(for: past, now: now, calendar: cal))
    }

    func testIdChangesWithContentOnly() {
        let a = PlannedNotification(identifier: "dsl.a1.wake", title: "起床", body: "", trigger: .daily(hour: 7, minute: 0), alarm: true)
        var b = a
        b.trigger = .daily(hour: 7, minute: 30)
        let ra = AlarmPlanner.requests([a], now: now, calendar: cal)[0]
        XCTAssertEqual(ra.id, AlarmPlanner.requests([a], now: now.addingTimeInterval(3600), calendar: cal)[0].id, "同じ中身は同じ id(予約し直さない)")
        XCTAssertNotEqual(ra.id, AlarmPlanner.requests([b], now: now, calendar: cal)[0].id, "時刻を変えたら別の id")
    }

    func testPlannedDecodesOldSavedEntries() throws {
        let old = #"{"identifier":"dsl.a1.n","title":"t","body":"","trigger":{"daily":{"hour":7,"minute":0}}}"#
        let p = try JSONDecoder().decode(PlannedNotification.self, from: Data(old.utf8))
        XCTAssertFalse(p.isAlarm)
    }

    // MARK: - 許可・落とし方

    func testAuthorizedAlarmsReplaceNotifications() async {
        let fake = FakeAlarms(authorized: true)
        let planned = [
            PlannedNotification(identifier: "dsl.a1.wake", title: "起床", body: "", trigger: .daily(hour: 7, minute: 0), alarm: true),
            PlannedNotification(identifier: "dsl.a1.memo", title: "記録", body: "", trigger: .daily(hour: 21, minute: 0)),
        ]
        let handled = await AlarmSync(alarms: fake).apply(planned, now: now, calendar: cal)
        XCTAssertEqual(handled, ["dsl.a1.wake"], "アラームで届ける分だけ普通の通知から外す")
        let ids = await fake.ids()
        XCTAssertEqual(ids.count, 1)
        // 2 回目は同じ id なので予約し直さない
        _ = await AlarmSync(alarms: fake).apply(planned, now: now, calendar: cal)
        let count = await fake.scheduleCount
        XCTAssertEqual(count, 1)
    }

    func testDeniedFallsBackToNotification() async {
        let fake = FakeAlarms(authorized: false)
        let planned = [PlannedNotification(identifier: "dsl.a1.wake", title: "起床", body: "", trigger: .daily(hour: 7, minute: 0), alarm: true)]
        let handled = await AlarmSync(alarms: fake).apply(planned, now: now, calendar: cal)
        XCTAssertTrue(handled.isEmpty, "許可が無ければ普通の通知で届ける")
        let ids = await fake.ids()
        XCTAssertTrue(ids.isEmpty)
    }

    func testScheduleFailureFallsBack() async {
        let fake = FakeAlarms(authorized: true, failSchedule: true)
        let planned = [PlannedNotification(identifier: "dsl.a1.wake", title: "起床", body: "", trigger: .daily(hour: 7, minute: 0), alarm: true)]
        let handled = await AlarmSync(alarms: fake).apply(planned, now: now, calendar: cal)
        XCTAssertTrue(handled.isEmpty, "上限などで予約できなければ普通の通知")
    }

    // MARK: - 後片付け

    func testStaleAlarmsCancelledButRingingKept() async {
        let fake = FakeAlarms(authorized: true)
        let wake = PlannedNotification(identifier: "dsl.a1.wake", title: "起床", body: "", trigger: .daily(hour: 7, minute: 0), alarm: true)
        _ = await AlarmSync(alarms: fake).apply([wake], now: now, calendar: cal)
        let ringing = UUID()
        await fake.add(ringing, alerting: true)
        // 通知の定義から消えた → 予約は消すが、今鳴っているものは止めない
        let handled = await AlarmSync(alarms: fake).apply([], now: now, calendar: cal)
        XCTAssertTrue(handled.isEmpty)
        let ids = await fake.ids()
        XCTAssertEqual(ids, [ringing])
        let asked = await fake.authorizeCount
        XCTAssertEqual(asked, 1, "アラームが無い時は許可を求めない")
    }

    func testRemoveAppCancelsItsAlarmsEvenRinging() async {
        let fake = FakeAlarms(authorized: true)
        let a1 = PlannedNotification(identifier: "dsl.a1.wake", title: "起床", body: "", trigger: .daily(hour: 7, minute: 0), alarm: true)
        let a2 = PlannedNotification(identifier: "dsl.a2.wake", title: "起床", body: "", trigger: .daily(hour: 6, minute: 0), alarm: true)
        let sync = AlarmSync(alarms: fake)
        _ = await sync.apply([a1, a2], now: now, calendar: cal)
        let id1 = AlarmPlanner.requests([a1], now: now, calendar: cal)[0].id
        await fake.setAlerting(id1)
        await sync.removeApp([a1], now: now)
        let ids = await fake.ids()
        XCTAssertEqual(ids, [AlarmPlanner.requests([a2], now: now, calendar: cal)[0].id], "削除したアプリの分だけ消す(鳴っていても)")
    }

    func testRemoveAllCancelsEverything() async {
        let fake = FakeAlarms(authorized: true)
        await fake.add(UUID(), alerting: true)
        await fake.add(UUID(), alerting: false)
        await AlarmSync(alarms: fake).removeAll()
        let ids = await fake.ids()
        XCTAssertTrue(ids.isEmpty, "サインアウト・アカウント削除で全部消す")
    }
}

/// AlarmKit の代わり(予約を手元に持つだけ)。
private actor FakeAlarms: AlarmScheduling {
    struct Failure: Error {}
    let authorized: Bool
    let failSchedule: Bool
    private var table: [UUID: Bool] = [:]
    private(set) var scheduleCount = 0
    private(set) var authorizeCount = 0

    init(authorized: Bool, failSchedule: Bool = false) {
        self.authorized = authorized
        self.failSchedule = failSchedule
    }

    func authorize() async -> Bool {
        authorizeCount += 1
        return authorized
    }

    func scheduled() async -> [ScheduledAlarm] { table.map { ScheduledAlarm(id: $0.key, alerting: $0.value) } }

    func schedule(_ request: AlarmRequest) async throws {
        if failSchedule { throw Failure() }
        scheduleCount += 1
        table[request.id] = false
    }

    func cancel(_ id: UUID) async { table[id] = nil }

    func add(_ id: UUID, alerting: Bool) { table[id] = alerting }
    func setAlerting(_ id: UUID) { table[id] = true }
    func ids() -> Set<UUID> { Set(table.keys) }
}
