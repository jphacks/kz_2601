import XCTest
import MetamonServices
@testable import Metamon

/// 「次の通知」の一覧(#228・#1325 `Shared/Notifications/UpcomingNotificationList.swift`・iOS の設定シートと Mac のポップオーバー)。
final class UpcomingNotificationListTests: XCTestCase {
    private let now = Date(timeIntervalSince1970: 1_800_000_000)

    private func item(_ title: String, body: String = "", in seconds: TimeInterval) -> UpcomingNotification {
        UpcomingNotification(title: title, body: body, date: now.addingTimeInterval(seconds))
    }

    func testKeepsOrderAndCopiesTitleBodyAndTime() {
        let upcoming = [item("水やり", body: "ベランダの鉢", in: 60), item("日記", in: 3 * 86_400)]
        let list = UpcomingNotificationList(upcoming, now: now)
        XCTAssertFalse(list.isEmpty)
        XCTAssertEqual(list.rows.map(\.title), ["水やり", "日記"])
        XCTAssertEqual(list.rows.map(\.body), ["ベランダの鉢", ""])
        XCTAssertEqual(list.rows.map(\.when), upcoming.map { NotificationCopy.when($0.date, now: now) })
        XCTAssertEqual(list.rows.map(\.id), [0, 1])
    }

    /// 先頭の 5 件まで(iOS の設定シートと同じ)
    func testShowsAtMostFive() {
        let upcoming = (0..<8).map { item("n\($0)", in: TimeInterval($0 + 1) * 3_600) }
        let list = UpcomingNotificationList(upcoming, now: now)
        XCTAssertEqual(list.rows.count, UpcomingNotificationList.limit)
        XCTAssertEqual(list.rows.map(\.title), ["n0", "n1", "n2", "n3", "n4"])
    }

    func testEmptyWhenNothingScheduled() {
        let list = UpcomingNotificationList([], now: now)
        XCTAssertTrue(list.isEmpty)
        XCTAssertTrue(list.rows.isEmpty)
    }

    /// 場所を見張っている・位置情報の案内を出す時は「予約中の通知はありません」を出さない(iOS)
    func testPlacesHideEmptyMessage() {
        XCTAssertFalse(UpcomingNotificationList([], places: 2, now: now).isEmpty)
        XCTAssertFalse(UpcomingNotificationList([], locationOff: true, now: now).isEmpty)
    }
}
