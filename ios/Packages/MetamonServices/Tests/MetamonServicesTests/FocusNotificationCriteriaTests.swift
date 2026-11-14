import XCTest
import UserNotifications
import MetamonDSL
@testable import MetamonServices

/// 集中モードのフィルタで OS が絞れるよう、生成アプリの通知の `filterCriteria` に生成アプリの id を入れる(#1992)。
final class FocusNotificationCriteriaTests: XCTestCase {
    private actor FakeCenter: NotificationCenterClient {
        private(set) var added: [String: String?] = [:]
        func pendingIdentifiers() async -> [String] { Array(added.keys) }
        func add(_ request: UNNotificationRequest) async throws { added[request.identifier] = request.content.filterCriteria }
        func removePending(_ identifiers: [String]) async { identifiers.forEach { added[$0] = nil } }
        func authorizationStatus() async -> UNAuthorizationStatus { .authorized }
        func requestAuthorization() async -> Bool { true }
    }

    private func scheduler() -> (UserNotificationScheduler, FakeCenter) {
        let suite = "focus-criteria-tests.\(UUID().uuidString)"
        let defaults = UserDefaults(suiteName: suite)!
        let center = FakeCenter()
        let s = UserNotificationScheduler(
            center: center, registry: NotificationRegistry(defaults: defaults), regionStore: RegionNotificationStore(defaults: defaults),
            location: RegionAccess(authorize: { false }, isAuthorized: { false }, lastKnown: { nil }), queue: AsyncSerialQueue())
        return (s, center)
    }

    func testScheduledNotificationsCarryTheirAppId() async throws {
        let (s, center) = scheduler()
        let daily = NotificationSpec(id: "wake", title: "起床", repeat: .daily, time: "06:00")
        _ = await s.sync(appId: "a.1", specs: [daily], records: [:])
        _ = await s.sync(appId: "b", specs: [daily], records: [:])
        _ = try await s.schedule(NotificationSpec(id: "tea", title: "お茶", repeat: .daily, time: "15:00"), appId: "a.1")
        let added = await center.added
        XCTAssertEqual(added["dsl.a.1.wake"], "a.1", "id に . があっても予約した側の id")
        XCTAssertEqual(added["dsl.b.wake"], "b")
        XCTAssertEqual(added["dsl.a.1.action.tea"], "a.1", "ボタンで予約した分も")
    }

    func testPostedNotificationCarriesAppId() async {
        let (s, center) = scheduler()
        _ = await s.post(NotificationSpec(id: "low", title: "残り少ない"), appId: "a1")
        let added = await center.added
        XCTAssertEqual(added[ConditionNotifications.identifier(appId: "a1", specId: "low")], "a1")
    }

    func testRequestWithoutAppIdHasNoCriteria() {
        let p = PlannedNotification(identifier: "dsl.x.y", title: "", body: "", trigger: .daily(hour: 7, minute: 0))
        XCTAssertNil(UserNotificationScheduler.request(p).content.filterCriteria)
        XCTAssertEqual(UserNotificationScheduler.request(p, appId: "x").content.filterCriteria, "x")
    }
}
