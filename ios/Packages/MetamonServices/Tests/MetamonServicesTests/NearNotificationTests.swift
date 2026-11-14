import XCTest
import UserNotifications
#if os(iOS)
import CoreLocation
#endif
import MetamonDSL
@testable import MetamonServices

/// 場所に着いたら知らせる(near)と時刻のずらし(jitterMinutes)。
final class NearNotificationTests: XCTestCase {
    private let cal: Calendar = {
        var c = Calendar(identifier: .gregorian)
        c.timeZone = TimeZone(identifier: "Asia/Tokyo")!
        return c
    }()
    private lazy var now = cal.date(from: DateComponents(year: 2026, month: 11, day: 8, hour: 12))!

    private func station(_ id: String, _ loc: String?, updated: String? = nil) -> Record {
        Record(id: id, data: ["name": .string(id), "spot": loc.map(JSONValue.string) ?? .null], updatedAt: updated)
    }

    private let nearSpec = NotificationSpec(id: "arrive", title: "もうすぐ到着", body: "降りる準備を", repeat: .once,
                                            near: .init(entity: "stations", locationField: "spot", radiusM: 800))

    func testDecodesNearAndJitter() throws {
        let json = #"{"id":"arrive","title":"t","body":"","repeat":"daily","near":{"entity":"stations","locationField":"spot"},"jitterMinutes":15}"#
        let spec = try JSONDecoder().decode(NotificationSpec.self, from: Data(json.utf8))
        XCTAssertEqual(spec.near, .init(entity: "stations", locationField: "spot"))
        XCTAssertEqual(spec.near?.radius, 200) // 既定
        XCTAssertEqual(spec.jitterMinutes, 15)
        XCTAssertTrue(spec.isNearBased)
        XCTAssertFalse(spec.isRecordBased)
        XCTAssertEqual(spec.recordEntity, "stations")
    }

    func testPlansOneRegionPerRecordWithPlace() {
        let recs = [station("kanazawa", "36.578,136.648"), station("empty", nil), station("broken", "abc")]
        let p = NotificationPlanner.plan(appId: "a1", specs: [nearSpec], records: ["stations": recs], now: now, calendar: cal)
        XCTAssertEqual(p.map(\.identifier), ["dsl.a1.arrive.kanazawa"])
        XCTAssertEqual(p.first?.trigger, .region(latitude: 36.578, longitude: 136.648, radius: 800, repeats: false))
        XCTAssertNil(p.first?.nextFire(after: now), "場所の通知は時刻の予約に混ぜない")
        XCTAssertTrue(NotificationPlanner.select(p, now: now).isEmpty)
        // daily = 着くたび
        var every = nearSpec
        every.repeat = .daily
        let d = NotificationPlanner.plan(appId: "a1", specs: [every], records: ["stations": recs], now: now, calendar: cal)
        XCTAssertEqual(d.first?.trigger, .region(latitude: 36.578, longitude: 136.648, radius: 800, repeats: true))
        // scheduleNotification のボタンには付けない
        XCTAssertNil(NotificationPlanner.action(appId: "a1", spec: nearSpec, now: now, calendar: cal))
    }

    /// 上限: 現在地が分かれば近い順、分からなければ新しく保存した順に 15 か所。
    func testSelectsNearestOrNewestRegions() {
        let recs = (0..<20).map { i in
            station("s\(i)", String(format: "%.3f,136.600", 36.0 + Double(i) * 0.01), updated: String(format: "2026-11-%02dT00:00:00Z", i + 1))
        }
        let p = NotificationPlanner.plan(appId: "a1", specs: [nearSpec], records: ["stations": recs], now: now, calendar: cal)
        XCTAssertEqual(p.count, 20)
        let newest = NotificationPlanner.selectRegions(p)
        XCTAssertEqual(newest.count, NotificationPlanner.regionLimit)
        XCTAssertEqual(newest.first?.identifier, "dsl.a1.arrive.s19")
        XCTAssertFalse(newest.contains { $0.identifier == "dsl.a1.arrive.s0" })
        let nearest = NotificationPlanner.selectRegions(p, here: (36.0, 136.6), limit: 3)
        XCTAssertEqual(nearest.map(\.identifier), ["dsl.a1.arrive.s0", "dsl.a1.arrive.s1", "dsl.a1.arrive.s2"])
    }

    /// 場所の通知は時刻が無いが、登録簿の掃除(過ぎた 1 回きり)で消さない。
    func testRegistryKeepsRegions() async {
        let defaults = UserDefaults(suiteName: "near-registry-tests")!
        defaults.removePersistentDomain(forName: "near-registry-tests")
        let reg = NotificationRegistry(defaults: defaults)
        await reg.set("a1", NotificationPlanner.plan(appId: "a1", specs: [nearSpec], records: ["stations": [station("k", "36.5,136.6")]], now: now, calendar: cal))
        let kept = await reg.pruned(now: now).map(\.identifier)
        XCTAssertEqual(kept, ["dsl.a1.arrive.k"])
    }

    /// 1 回きり: 予約した後に iOS の予約から消えていれば届いたとみなし、予約し直さない。場所が変われば見張り直す。
    func testOnceRegionIsNotRearmedAfterFiring() async {
        let defaults = UserDefaults(suiteName: "near-store-tests")!
        defaults.removePersistentDomain(forName: "near-store-tests")
        let store = RegionNotificationStore(defaults: defaults)
        let plan = { (loc: String) in
            NotificationPlanner.plan(appId: "a1", specs: [self.nearSpec], records: ["stations": [self.station("k", loc)]], now: self.now, calendar: self.cal)
        }
        let first = plan("36.5,136.6")
        let pick1 = await store.toSchedule(first, known: first, pending: [])
        XCTAssertEqual(pick1.count, 1)
        await store.setArmed(pick1)
        let pick2 = await store.toSchedule(first, known: first, pending: ["dsl.a1.arrive.k"]) // まだ予約に残っている
        XCTAssertEqual(pick2.count, 1)
        await store.setArmed(pick2)
        let pick3 = await store.toSchedule(first, known: first, pending: []) // 届いて予約から消えた
        XCTAssertTrue(pick3.isEmpty)
        await store.setArmed(pick3)
        let reloaded = RegionNotificationStore(defaults: defaults)
        let pick4 = await reloaded.toSchedule(first, known: first, pending: [])
        XCTAssertTrue(pick4.isEmpty, "アプリを閉じても 1 回きりを守る")
        let moved = plan("36.6,136.7")
        let pick5 = await reloaded.toSchedule(moved, known: moved, pending: [])
        XCTAssertEqual(pick5.count, 1, "場所を直したら見張り直す")
        // 着くたび(daily)は届いても予約し直す
        var every = nearSpec
        every.repeat = .daily
        let d = NotificationPlanner.plan(appId: "a1", specs: [every], records: ["stations": [station("k", "36.5,136.6")]], now: now, calendar: cal)
        await reloaded.setArmed(d)
        let again = await reloaded.toSchedule(d, known: d, pending: [])
        XCTAssertEqual(again.count, 1)
    }

    #if os(iOS)
    func testRegionRequestUsesLocationTrigger() {
        let p = NotificationPlanner.plan(appId: "a1", specs: [nearSpec], records: ["stations": [station("k", "36.5,136.6")]], now: now, calendar: cal)
        let t = UserNotificationScheduler.request(p[0]).trigger as? UNLocationNotificationTrigger
        let region = t?.region as? CLCircularRegion
        XCTAssertEqual(region?.radius, 800)
        XCTAssertEqual(region?.center.latitude, 36.5)
        XCTAssertEqual(region?.notifyOnEntry, true)
        XCTAssertEqual(region?.notifyOnExit, false)
        XCTAssertEqual(t?.repeats, false)
    }
    #endif

    // MARK: - jitterMinutes

    func testJitterSpreadsDailyOccurrencesStably() {
        let spec = NotificationSpec(id: "photo", title: "写真の時間", repeat: .daily, time: "20:00", jitterMinutes: 60)
        let p = NotificationPlanner.plan(appId: "a1", specs: [spec], records: [:], now: now, calendar: cal)
        XCTAssertEqual(p.count, NotificationPlanner.jitterDailyAhead)
        let fires = p.compactMap { if case .at(let d) = $0.trigger { d } else { nil } }
        XCTAssertEqual(fires.count, p.count, "回ごとに具体的な日時で予約する")
        for (i, f) in fires.enumerated() {
            let base = cal.date(byAdding: .day, value: i, to: cal.date(from: DateComponents(year: 2026, month: 11, day: 8, hour: 20))!)!
            XCTAssertLessThanOrEqual(abs(f.timeIntervalSince(base)), 3600, "±60 分以内")
        }
        XCTAssertGreaterThan(Set(fires.map { cal.component(.minute, from: $0) }).count, 1, "毎回同じずれにしない")
        XCTAssertEqual(Set(p.map(\.identifier)).count, p.count)
        XCTAssertTrue(p.allSatisfy { $0.identifier.hasPrefix("dsl.a1.photo.j") }, "取消の前方一致で消える")
        // 予約し直しても同じ回は同じ時刻
        let later = NotificationPlanner.plan(appId: "a1", specs: [spec], records: [:], now: now.addingTimeInterval(3600), calendar: cal)
        XCTAssertEqual(later.first?.trigger, p.first?.trigger)
        XCTAssertEqual(later.first?.identifier, p.first?.identifier)
    }

    /// 内緒の前倒し: offsetMinutes -30 + jitterMinutes 30 → 期限の 0〜60 分前。
    func testJitterWithNegativeOffsetIsAlwaysEarly() {
        let spec = NotificationSpec(id: "due", title: "期限", repeat: .once, entity: "tasks", field: "due", offsetMinutes: -30, jitterMinutes: 30)
        let dueText = "2026-11-10T18:00:00+09:00"
        let due = FieldCoding.date(from: .string(dueText))!
        let recs = (0..<10).map { Record(id: "t\($0)", data: ["due": .string(dueText)]) }
        let p = NotificationPlanner.plan(appId: "a1", specs: [spec], records: ["tasks": recs], now: now, calendar: cal)
        XCTAssertEqual(p.count, 10)
        for n in p {
            guard case .at(let d) = n.trigger else { return XCTFail("once は具体的な日時") }
            XCTAssertLessThanOrEqual(d, due)
            XCTAssertGreaterThanOrEqual(d, due.addingTimeInterval(-3600))
        }
        XCTAssertEqual(p.map(\.identifier), recs.map { "dsl.a1.due.\($0.id)" }, "1 回きりは識別子を変えない")
    }

    func testNoJitterKeepsRepeatingTrigger() {
        let spec = NotificationSpec(id: "m", title: "朝", repeat: .daily, time: "07:00")
        let p = NotificationPlanner.plan(appId: "a1", specs: [spec], records: [:], now: now, calendar: cal)
        XCTAssertEqual(p.map(\.trigger), [.daily(hour: 7, minute: 0)])
    }
}
