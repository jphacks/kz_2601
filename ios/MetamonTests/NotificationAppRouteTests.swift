import XCTest
@testable import Metamon

/// 押された通知からアプリを引く(#965 `Shared/Notifications/NotificationCopy.swift`・Mac が使う)。
final class NotificationAppRouteTests: XCTestCase {
    private let ids = ["habit", "habit.v2", "memo"]

    func testScheduledAndActionIdentifiers() {
        XCTAssertEqual(NotificationAppRoute.appId(for: "dsl.memo.daily", among: ids), "memo")
        XCTAssertEqual(NotificationAppRoute.appId(for: "dsl.memo.action.remind", among: ids), "memo")
        XCTAssertEqual(NotificationAppRoute.appId(for: "dsl.memo.due.rec-1", among: ids), "memo")
    }

    func testConditionAndTimerIdentifiers() {
        XCTAssertEqual(NotificationAppRoute.appId(for: "when.habit.streak", among: ids), "habit")
        XCTAssertEqual(NotificationAppRoute.appId(for: "timer.memo/home/t1", among: ids), "memo")
    }

    /// appId に「.」が入っても、一番長く合うアプリを選ぶ
    func testPrefersLongestAppId() {
        XCTAssertEqual(NotificationAppRoute.appId(for: "dsl.habit.v2.daily", among: ids), "habit.v2")
        XCTAssertEqual(NotificationAppRoute.appId(for: "dsl.habit.daily", among: ids), "habit")
    }

    func testUnknownIdentifiers() {
        XCTAssertNil(NotificationAppRoute.appId(for: "dsl.other.daily", among: ids))
        XCTAssertNil(NotificationAppRoute.appId(for: "push.memo", among: ids))
        XCTAssertNil(NotificationAppRoute.appId(for: "timer.memo.home", among: ids)) // タイマーは「/」区切り
    }
}
