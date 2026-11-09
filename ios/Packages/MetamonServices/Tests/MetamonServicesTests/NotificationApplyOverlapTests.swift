import XCTest
import UserNotifications
import MetamonDSL
@testable import MetamonServices

/// 予約し直し(apply)が重なっても、1 回きりの場所の通知を「届いた」と誤らない(PR #720 のレビュー)。
final class NotificationApplyOverlapTests: XCTestCase {
    /// 予約一覧の取得・追加の間に他の処理が割り込めるよう、毎回 yield する偽の通知センター。
    private actor FakeCenter: NotificationCenterClient {
        private var pending: Set<String> = []
        private(set) var log: [String] = []

        func pendingIdentifiers() async -> [String] {
            let snapshot = Array(pending)
            log.append("pending")
            for _ in 0..<20 { await Task.yield() }
            return snapshot
        }

        func add(_ request: UNNotificationRequest) async throws {
            for _ in 0..<20 { await Task.yield() }
            pending.insert(request.identifier)
            log.append("add:" + request.identifier)
        }

        func removePending(_ identifiers: [String]) async { pending.subtract(identifiers) }
        func authorizationStatus() async -> UNAuthorizationStatus { .authorized }
        func requestAuthorization() async -> Bool { true }
        func clearLog() { log = [] }
    }

    private let nearSpec = NotificationSpec(id: "arrive", title: "着いた", repeat: .once,
                                            near: .init(entity: "stations", locationField: "spot", radiusM: 300))

    private func scheduler(_ suite: String) async -> (UserNotificationScheduler, FakeCenter) {
        let defaults = UserDefaults(suiteName: suite)!
        defaults.removePersistentDomain(forName: suite)
        let center = FakeCenter()
        let s = UserNotificationScheduler(
            center: center, registry: NotificationRegistry(defaults: defaults), regionStore: RegionNotificationStore(defaults: defaults),
            location: RegionAccess(authorize: { true }, isAuthorized: { true }, lastKnown: { nil }), queue: AsyncSerialQueue())
        return (s, center)
    }

    func testOverlappingAppliesDoNotMarkOnceRegionFired() async {
        let (s, center) = await scheduler("apply-overlap-tests")
        let rec = Record(id: "k", data: ["spot": .string("36.5,136.6")])
        _ = await s.sync(appId: "a1", specs: [nearSpec], records: ["stations": [rec]])
        await center.clearLog()

        async let r1 = s.refresh(appId: "a1")
        async let r2 = s.refresh(appId: "a1")
        let (a, b) = await (r1, r2)
        XCTAssertEqual(a.places, 1)
        XCTAssertEqual(b.places, 1)
        let log = await center.log
        XCTAssertEqual(log, ["pending", "add:dsl.a1.arrive.k", "pending", "add:dsl.a1.arrive.k"], "重ならず 1 本ずつ")

        // 予約に残っている間は届いていない: 次も見張り続ける
        let again = await s.refresh(appId: "a1")
        XCTAssertEqual(again.places, 1)
    }

    func testSerialQueueRunsOneAtATime() async {
        let q = AsyncSerialQueue()
        actor Counter {
            var now = 0, peak = 0
            func enter() { now += 1; peak = max(peak, now) }
            func leave() { now -= 1 }
        }
        let c = Counter()
        await withTaskGroup(of: Void.self) { g in
            for _ in 0..<8 {
                g.addTask {
                    await q.run {
                        await c.enter()
                        for _ in 0..<10 { await Task.yield() }
                        await c.leave()
                    }
                }
            }
        }
        let peak = await c.peak
        XCTAssertEqual(peak, 1)
    }
}
