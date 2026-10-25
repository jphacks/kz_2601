import XCTest
import MetamonDSL
@testable import MetamonServices

/// #228: 通知の予約の組み立て(時刻・繰り返し・レコードからの通知・上限)。
final class NotificationPlannerTests: XCTestCase {
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

    func testFixedDailyWeeklyOnce() {
        let specs = [
            NotificationSpec(id: "wake", title: "起床", repeat: .daily, time: "07:00"),
            NotificationSpec(id: "gym", title: "ジム", repeat: .weekly, time: "19:30", weekday: "mon"),
            NotificationSpec(id: "noon", title: "昼", repeat: .once, time: "12:00"),
        ]
        let p = NotificationPlanner.plan(appId: "a1", specs: specs, records: [:], now: now, calendar: cal)
        XCTAssertEqual(p.map(\.identifier), ["dsl.a1.wake", "dsl.a1.gym", "dsl.a1.noon"])
        XCTAssertEqual(p[0].trigger, .daily(hour: 7, minute: 0))
        XCTAssertEqual(p[1].trigger, .weekly(weekday: 2, hour: 19, minute: 30))
        XCTAssertEqual(p[2].trigger, .at(at(25, 12, 0)))
        XCTAssertEqual(p[0].nextFire(after: now, calendar: cal), at(26, 7, 0)) // 今日の 7:00 は過ぎた → 明日
        XCTAssertEqual(p[1].nextFire(after: now, calendar: cal), at(26, 19, 30)) // 10/26 は月曜
    }

    func testRecordAlarmTimeDailyWithEnabledFlag() {
        let spec = NotificationSpec(id: "alarm", title: "アラーム", repeat: .daily, entity: "alarm", field: "time", titleField: "label", enabledField: "on")
        let recs = ["alarm": [
            Record(id: "r1", data: ["time": "06:45", "label": "起きる", "on": .bool(true)]),
            Record(id: "r2", data: ["time": "08:00", "label": "休日", "on": .bool(false)]),
            Record(id: "r3", data: ["time": "22:10"]), // on 未設定 = 有効
        ]]
        let p = NotificationPlanner.plan(appId: "a1", specs: [spec], records: recs, now: now, calendar: cal)
        XCTAssertEqual(p.map(\.identifier), ["dsl.a1.alarm.r1", "dsl.a1.alarm.r3"])
        XCTAssertEqual(p[0].trigger, .daily(hour: 6, minute: 45))
        XCTAssertEqual(p[0].body, "起きる")
        XCTAssertEqual(p[1].trigger, .daily(hour: 22, minute: 10))
    }

    func testRecordDatetimeTenMinutesBefore() {
        let spec = NotificationSpec(id: "before", title: "まもなく予定", body: "", repeat: .once, entity: "event", field: "when", offsetMinutes: -10, titleField: "title")
        let iso = ISO8601DateFormatter()
        let recs = ["event": [
            Record(id: "e1", data: ["when": .string(iso.string(from: at(25, 15, 0))), "title": "会議"]),
            Record(id: "e2", data: ["when": .string(iso.string(from: at(25, 10, 5)))]), // 10 分前は過ぎた
            Record(id: "e3", data: ["when": "2026-10-27", "title": "締切"]),            // 日付だけ = 9:00 の 10 分前
            Record(id: "e4", data: ["when": .null]),
        ]]
        let p = NotificationPlanner.plan(appId: "a1", specs: [spec], records: recs, now: now, calendar: cal)
        XCTAssertEqual(p.map(\.identifier), ["dsl.a1.before.e1", "dsl.a1.before.e3"])
        XCTAssertEqual(p[0].trigger, .at(at(25, 14, 50)))
        XCTAssertEqual(p[0].body, "会議")
        XCTAssertEqual(p[1].nextFire(after: now, calendar: cal), cal.date(from: DateComponents(year: 2026, month: 10, day: 27, hour: 8, minute: 50)))
    }

    func testOffsetWrapsAroundMidnightForTimeOnly() {
        let spec = NotificationSpec(id: "a", title: "t", repeat: .daily, entity: "x", field: "time", offsetMinutes: -30)
        let p = NotificationPlanner.plan(appId: "a1", specs: [spec], records: ["x": [Record(id: "r", data: ["time": "00:10"])]], now: now, calendar: cal)
        XCTAssertEqual(p.first?.trigger, .daily(hour: 23, minute: 40))
    }

    func testSelectKeepsNearestWithinLimitAndDropsPast() {
        let past = PlannedNotification(identifier: "dsl.a.past", title: "", body: "", trigger: .at(at(25, 9, 0)))
        let many = (0..<80).map { i in
            PlannedNotification(identifier: "dsl.a.n\(i)", title: "", body: "", trigger: .at(now.addingTimeInterval(TimeInterval((80 - i) * 3600))))
        }
        let daily = PlannedNotification(identifier: "dsl.b.d", title: "", body: "", trigger: .daily(hour: 10, minute: 30))
        let chosen = NotificationPlanner.select([past] + many + [daily], now: now, calendar: cal, limit: 60)
        XCTAssertEqual(chosen.count, 60)
        XCTAssertFalse(chosen.contains { $0.0.identifier == "dsl.a.past" })
        XCTAssertEqual(chosen.first?.0.identifier, "dsl.b.d") // 30 分後が最も近い
        XCTAssertEqual(chosen[1].0.identifier, "dsl.a.n79")  // 1 時間後
        XCTAssertTrue(zip(chosen, chosen.dropFirst()).allSatisfy { $0.1 <= $1.1 })
        XCTAssertFalse(chosen.contains { $0.0.identifier == "dsl.a.n0" }) // 80 時間後は上限外
    }

    func testActionUsesSeparateNamespace() {
        let spec = NotificationSpec(id: "wake", title: "起床", repeat: .daily, time: "06:00")
        XCTAssertEqual(NotificationPlanner.action(appId: "a1", spec: spec, now: now, calendar: cal)?.identifier, "dsl.a1.action.wake")
    }

    func testRegistryKeepsOtherAppsAndActions() async {
        let defaults = UserDefaults(suiteName: "notif228-tests")!
        defaults.removePersistentDomain(forName: "notif228-tests")
        let reg = NotificationRegistry(defaults: defaults)
        let a = PlannedNotification(identifier: "dsl.a.x", title: "", body: "", trigger: .daily(hour: 7, minute: 0))
        let b = PlannedNotification(identifier: "dsl.b.x", title: "", body: "", trigger: .daily(hour: 8, minute: 0))
        let act = PlannedNotification(identifier: "dsl.a.action.y", title: "", body: "", trigger: .daily(hour: 6, minute: 0))
        await reg.set("a", [a])
        await reg.set("b", [b])
        await reg.upsertAction("a", act)
        await reg.set("a", []) // 再生成で app.notifications が消えてもボタンの分と別アプリは残る
        let ids = Set(await reg.pruned(now: Date()).map(\.identifier))
        XCTAssertEqual(ids, ["dsl.b.x", "dsl.a.action.y"])
        // 永続化されている
        let reloaded = NotificationRegistry(defaults: defaults)
        let reloadedIds = Set(await reloaded.pruned(now: Date()).map(\.identifier))
        XCTAssertEqual(reloadedIds, ids)
        await reloaded.removeApp("b")
        let afterRemove = await reloaded.pruned(now: Date()).map(\.identifier)
        XCTAssertEqual(afterRemove, ["dsl.a.action.y"])
    }

    func testDecodesRecordBasedSpec() throws {
        let json = #"{"id":"a","title":"t","body":"","repeat":"once","entity":"event","field":"when","offsetMinutes":-10,"titleField":"title"}"#
        let s = try JSONDecoder().decode(NotificationSpec.self, from: Data(json.utf8))
        XCTAssertTrue(s.isRecordBased)
        XCTAssertEqual(s.offsetMinutes, -10)
    }
}
