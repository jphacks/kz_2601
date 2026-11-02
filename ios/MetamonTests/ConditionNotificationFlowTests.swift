import XCTest
import MetamonDSL
import MetamonServices
@testable import Metamon

/// 条件で知らせる通知(#265)の完了の条件: 出荷を記録して在庫が閾値を下回ると、その場で通知が出る。
@MainActor
final class ConditionNotificationFlowTests: XCTestCase {
    private let appId = "test-when-\(UUID().uuidString.lowercased())"

    override func tearDown() async throws {
        await ConditionNotificationStore.shared.removeApp(appId)
    }

    /// 出した通知を記録するだけ(UNUserNotificationCenter に触らない)。
    final class SpyScheduler: NotificationScheduler, @unchecked Sendable {
        private let lock = NSLock()
        private var _posted: [String] = []
        var posted: [String] { lock.withLock { _posted } }
        func sync(appId: String, specs: [NotificationSpec], records: [String: [Record]]) async -> NotificationSyncResult { .init() }
        func schedule(_ spec: NotificationSpec, appId: String) async throws -> Date? { nil }
        func cancel(id: String, appId: String) async {}
        func post(_ spec: NotificationSpec, appId: String) async -> Bool {
            lock.withLock { _posted.append(spec.id) }
            return true
        }
        func removeApp(_ appId: String) async {}
        func removeAll() async {}
        func refresh(appId: String) async -> NotificationSyncResult { .init() }
    }

    func testShipmentBelowThresholdPostsRightAway() async throws {
        let json = """
        {"schemaVersion":1,"app":{"name":"在庫","voiceEntry":"floating","notifications":[
          {"id":"low","title":"在庫が少なくなりました","body":"","repeat":"once",
           "when":{"op":"lt","args":[{"agg":"sum","entity":"moves","field":"qty"},5]}}]},
         "initialScreen":"home",
         "entities":[{"id":"moves","name":"入出荷","fields":[{"id":"qty","name":"数","type":"integer"}]}],
         "screens":[{"id":"home","title":"在庫","components":[{"type":"list","entity":"moves"}]}]}
        """
        let dsl = try JSONDecoder().decode(AppDSL.self, from: Data(json.utf8))
        var services = BuildConfig.make(environment: .mock)
        let spy = SpyScheduler()
        services.notifications = spy
        let rt = AppRuntime(document: AppDocument(appId: appId, version: 1, dsl: dsl), services: services)

        _ = await rt.createRecord(entity: "moves", data: ["qty": .number(10)]) // 入荷 10
        await rt.checkConditionNotificationsNow()
        XCTAssertEqual(spy.posted, [], "在庫 10 は閾値の上")

        _ = await rt.createRecord(entity: "moves", data: ["qty": .number(-7)]) // 出荷 → 3
        await rt.checkConditionNotificationsNow()
        XCTAssertEqual(spy.posted, ["low"], "下回った保存の直後に出る")

        _ = await rt.createRecord(entity: "moves", data: ["qty": .number(-1)]) // 2: 下回ったまま
        await rt.checkConditionNotificationsNow()
        XCTAssertEqual(spy.posted, ["low"], "真のままの保存では出さない")
    }
}
