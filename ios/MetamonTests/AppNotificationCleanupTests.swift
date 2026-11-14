import XCTest
import MetamonDSL
import MetamonRenderer
import MetamonServices
@testable import Metamon

/// アプリを消した時の通知の後片付け(#1324・iOS と Mac で共有)。
/// 消したアプリの予約とタイマーだけを消し、別のアプリ(id の頭が同じでも)は残す。
@MainActor
final class AppNotificationCleanupTests: XCTestCase {
    private let deleted = "cleanup-\(UUID().uuidString.lowercased())"
    private var other: String { deleted + "x" } // 頭が同じ別のアプリ

    override func tearDown() async throws {
        TimerRunStore.shared.clearApp(appId: deleted)
        TimerRunStore.shared.clearApp(appId: other)
    }

    final class SpyScheduler: NotificationScheduler, @unchecked Sendable {
        private let lock = NSLock()
        private var _removed: [String] = []
        var removed: [String] { lock.withLock { _removed } }
        func sync(appId: String, specs: [NotificationSpec], records: [String: [Record]]) async -> NotificationSyncResult { .init() }
        func schedule(_ spec: NotificationSpec, appId: String) async throws -> Date? { nil }
        func cancel(id: String, appId: String) async {}
        func post(_ spec: NotificationSpec, appId: String) async -> Bool { false }
        func removeApp(_ appId: String) async { lock.withLock { _removed.append(appId) } }
        func removeAll() async {}
        func refresh(appId: String) async -> NotificationSyncResult { .init() }
    }

    func testRemovesOnlyDeletedAppsNotificationsAndTimers() async {
        let scheduler = SpyScheduler()
        let mine = TimerRunStore.key(appId: deleted, screen: "home", component: "t")
        let theirs = TimerRunStore.key(appId: other, screen: "home", component: "t")
        TimerRunStore.shared.save(TimerRun(base: 5), for: mine)
        TimerRunStore.shared.save(TimerRun(base: 7), for: theirs)

        await AppNotificationCleanup.removeApp(deleted, notifications: scheduler)

        XCTAssertEqual(scheduler.removed, [deleted], "消したアプリの通知の予約だけを消す")
        XCTAssertNil(TimerRunStore.shared.load(mine), "消したアプリのタイマーは消える")
        XCTAssertEqual(TimerRunStore.shared.load(theirs), TimerRun(base: 7), "id の頭が同じ別のアプリのタイマーは残る")
    }
}
