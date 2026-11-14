import XCTest
import UserNotifications
@testable import MetamonRenderer

/// #38 の続き: タイマーのキーは appId(同じ名前のアプリで混ざらない)・古いキーからの移行・終了通知は開始時点で予約。
@MainActor
final class TimerKeysTests: XCTestCase {
    private var defaults: UserDefaults!
    private var suite = ""
    private let t0 = Date(timeIntervalSince1970: 1_000_000)
    private let idA = "aaaaaaaa-0000-0000-0000-000000000001"
    private let idB = "bbbbbbbb-0000-0000-0000-000000000002"
    private let idC = "cccccccc-0000-0000-0000-000000000003"

    override func setUp() {
        suite = "timer-keys-\(UUID().uuidString)"
        defaults = UserDefaults(suiteName: suite)
    }

    override func tearDown() {
        defaults.removePersistentDomain(forName: suite)
        defaults = nil
        TimerNotifier.center = nil
    }

    private func raw(_ key: String) -> Data? { defaults.data(forKey: TimerRunStore.prefix + key) }
    private func putOld(_ key: String, _ run: TimerRun) {
        defaults.set(try? JSONEncoder().encode(run), forKey: TimerRunStore.prefix + key)
    }

    // MARK: - 同じ名前の 2 つのアプリ

    func testSameNameAppsDoNotShareTimer() {
        let store = TimerRunStore(defaults: defaults)
        let a = TimerRunStore.key(appId: idA, screen: "home", component: "timer")
        let b = TimerRunStore.key(appId: idB, screen: "home", component: "timer")
        XCTAssertNotEqual(a, b, "名前が同じ「ラーメン」でも appId が違えば別のキー")
        store.save(TimerRun(base: 0, startedAt: t0), for: a)
        XCTAssertNil(store.load(b))
        store.save(TimerRun(base: 7), for: b)
        store.clearApp(appId: idB)
        XCTAssertEqual(store.load(a), TimerRun(base: 0, startedAt: t0), "片方を消してももう片方は残る")
        XCTAssertNil(store.load(b))
    }

    // MARK: - 古いキーからの移行

    func testMigratesOldNameKeyToAppIdOnceAndRemovesOldKey() {
        let run = TimerRun(base: 3, startedAt: t0)
        putOld("ラーメン/home/timer", run)
        let apps = [TimerKeyMigration.App(id: idA, name: "ラーメン")]

        let result = TimerKeyMigration.migrateStore(defaults, apps: apps, current: nil)

        XCTAssertEqual(result.moved, ["ラーメン/home/timer": "\(idA)/home/timer"])
        XCTAssertNil(raw("ラーメン/home/timer"), "古いキーは残さない")
        XCTAssertEqual(TimerRunStore(defaults: defaults).load("\(idA)/home/timer"), run)
        XCTAssertTrue(defaults.bool(forKey: TimerKeyMigration.doneKey), "移した印を残す")

        // 2 回目は何もしない(印があるので、後から入った古い形のキーも触らない)
        putOld("ラーメン/home/t2", run)
        XCTAssertEqual(TimerKeyMigration.migrateStore(defaults, apps: apps, current: nil), .init())
        XCTAssertNotNil(raw("ラーメン/home/t2"))
    }

    func testDuplicateNamesGoToOpenAppOrAreDropped() {
        let run = TimerRun(base: 0, startedAt: t0)
        putOld("ラーメン/home/timer", run)
        let apps = [TimerKeyMigration.App(id: idA, name: "ラーメン"), .init(id: idB, name: "ラーメン")]

        // 開いているアプリが同じ名前ならそちらに寄せる
        let result = TimerKeyMigration.migrateStore(defaults, apps: apps, current: apps[1])
        XCTAssertEqual(result.moved, ["ラーメン/home/timer": "\(idB)/home/timer"])
        XCTAssertNil(TimerRunStore(defaults: defaults).load("\(idA)/home/timer"))

        // 開いているのが別のアプリなら決められないので捨てる
        let other = UserDefaults(suiteName: suite + "-2")!
        defer { other.removePersistentDomain(forName: suite + "-2") }
        other.set(try? JSONEncoder().encode(run), forKey: TimerRunStore.prefix + "ラーメン/home/timer")
        let dropped = TimerKeyMigration.migrateStore(other, apps: apps, current: .init(id: idC, name: "家計簿"))
        XCTAssertEqual(dropped.dropped, ["ラーメン/home/timer"])
        XCTAssertTrue(dropped.moved.isEmpty)
        XCTAssertNil(other.data(forKey: TimerRunStore.prefix + "ラーメン/home/timer"), "捨てた分も古いキーは残さない")
    }

    func testUnknownNameDroppedAndNewKeysKeptAndLongestNameWins() {
        let run = TimerRun(base: 4)
        putOld("消えたアプリ/home/timer", run)
        putOld("\(idC)/home/timer", run) // 既に新しい形(一覧に無いアプリ)
        putOld("A/B/home/timer", run)   // 名前に / を含むアプリ
        let apps = [TimerKeyMigration.App(id: idA, name: "A"), .init(id: idB, name: "A/B")]

        let result = TimerKeyMigration.migrateStore(defaults, apps: apps, current: nil)

        XCTAssertEqual(result.dropped, ["消えたアプリ/home/timer"])
        XCTAssertEqual(result.moved, ["A/B/home/timer": "\(idB)/home/timer"])
        XCTAssertNotNil(raw("\(idC)/home/timer"), "新しい形のキーは触らない")
    }

    // MARK: - 終了通知

    func testScheduledAtStartWithoutScreenAndCancelledOnStop() async {
        let center = FakeTimerCenter()
        TimerNotifier.center = center
        let key = TimerRunStore.key(appId: idA, screen: "home", component: "timer")
        let now = Date()
        // 部品が一度も画面に出ていない(TimerVisibility に無い)状態で開始
        let plan = TimerNotificationPlan.decide(isCountdown: true, run: TimerRun(base: 0, startedAt: now), total: 300, now: now)
        TimerNotifier.apply(plan, key: key, title: "ラーメン", name: "麺")
        await TimerNotifier.drain()
        XCTAssertEqual(center.pending.map(\.identifier), ["timer.\(key)"])
        XCTAssertFalse(TimerVisibility.isShowing(notificationId: "timer.\(key)"), "見えていないのでバナーも出る")

        // 一時停止(止める)で取り消す
        let paused = TimerNotificationPlan.decide(isCountdown: true, run: TimerRun(base: 10), total: 300, now: now)
        TimerNotifier.apply(paused, key: key, title: "ラーメン", name: "麺")
        await TimerNotifier.drain()
        XCTAssertTrue(center.pending.isEmpty)
    }

    func testStartThenImmediateStopLeavesNothing() async {
        let center = FakeTimerCenter()
        TimerNotifier.center = center
        let now = Date()
        TimerNotifier.apply(.schedule(at: now + 60), key: "\(idA)/home/t", title: "x", name: "y")
        TimerNotifier.apply(.cancel, key: "\(idA)/home/t", title: "x", name: "y")
        await TimerNotifier.drain()
        XCTAssertTrue(center.pending.isEmpty, "予約と取り消しは順番どおり(後から予約が残らない)")
    }

    func testVisibleTimerSuppressesBanner() {
        let key = "\(idA)/home/visible"
        TimerVisibility.appeared(key)
        XCTAssertTrue(TimerVisibility.isShowing(notificationId: "timer.\(key)"))
        XCTAssertFalse(TimerVisibility.isShowing(notificationId: "dsl.\(key)"), "タイマー以外の通知は関係ない")
        TimerVisibility.disappeared(key)
        XCTAssertFalse(TimerVisibility.isShowing(notificationId: "timer.\(key)"))
    }

    func testDeleteAppCancelsOnlyThatAppsNotifications() async {
        let center = FakeTimerCenter()
        TimerNotifier.center = center
        let now = Date()
        TimerNotifier.apply(.schedule(at: now + 60), key: "\(idA)/home/t", title: "ラーメン", name: "y")
        TimerNotifier.apply(.schedule(at: now + 60), key: "\(idB)/home/t", title: "ラーメン", name: "y")
        await TimerCleanup.removeApp(appId: idA)
        XCTAssertEqual(center.pending.map(\.identifier), ["timer.\(idB)/home/t"], "同じ名前の別のアプリの予約は残る")
    }

    func testMigratesPendingNotificationIds() async {
        let center = FakeTimerCenter()
        TimerNotifier.center = center
        let trigger = UNTimeIntervalNotificationTrigger(timeInterval: 120, repeats: false)
        try? await center.add(UNNotificationRequest(identifier: "timer.ラーメン/home/timer", content: UNMutableNotificationContent(), trigger: trigger))
        try? await center.add(UNNotificationRequest(identifier: "timer.消えた/home/timer", content: UNMutableNotificationContent(), trigger: trigger))
        try? await center.add(UNNotificationRequest(identifier: "dsl.other", content: UNMutableNotificationContent(), trigger: trigger))

        var result = TimerKeyMigration.Result()
        result.moved = ["ラーメン/home/timer": "\(idA)/home/timer"]
        result.dropped = ["消えた/home/timer"]
        await TimerKeyMigration.migrateNotifications(result)

        XCTAssertEqual(Set(center.pending.map(\.identifier)), ["timer.\(idA)/home/timer", "dsl.other"])
    }
}

final class FakeTimerCenter: TimerNotificationCenter {
    var pending: [UNNotificationRequest] = []

    func add(_ request: UNNotificationRequest) async throws {
        pending.removeAll { $0.identifier == request.identifier }
        pending.append(request)
    }

    func removePendingNotificationRequests(withIdentifiers identifiers: [String]) {
        pending.removeAll { identifiers.contains($0.identifier) }
    }

    func pendingNotificationRequests() async -> [UNNotificationRequest] { pending }
    func requestAuthorizationIfNotDetermined() async {}
}
