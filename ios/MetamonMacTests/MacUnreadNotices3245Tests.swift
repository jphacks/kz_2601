#if !APP_STORE_REVIEW
import XCTest
import MetamonServices
@testable import Metamon

/// 未読の直ったお知らせの数(#3245)。サイドバーの設定の行と Dock のバッジの文字・読み上げ・読み直し。
@MainActor
final class MacUnreadNotices3245Tests: XCTestCase {
    private final class FakeClient: LoopNoticesClient, @unchecked Sendable {
        var unread = 0
        var fails = false
        var calls = 0
        func notices(pageToken: String?) async throws -> LoopNoticePage {
            calls += 1
            if fails { throw URLError(.notConnectedToInternet) }
            return LoopNoticePage(items: [], unreadCount: unread)
        }
        func markRead(id: String) async throws {}
        func feedbackFixedConsent() async throws -> Bool { false }
        func setFeedbackFixedConsent(_ enabled: Bool) async throws {}
    }

    func testBadgeLabel() {
        XCTAssertNil(MacUnreadNotices.badgeLabel(0), "0 なら出さない")
        XCTAssertNil(MacUnreadNotices.badgeLabel(-1))
        XCTAssertEqual(MacUnreadNotices.badgeLabel(1), "1")
        XCTAssertEqual(MacUnreadNotices.badgeLabel(99), "99")
        XCTAssertEqual(MacUnreadNotices.badgeLabel(100), "99+")
    }

    func testAccessibilityLabel() {
        XCTAssertNil(MacUnreadNotices.accessibilityLabel(0))
        let label = MacUnreadNotices.accessibilityLabel(3) ?? ""
        XCTAssertTrue(label.contains("3"), label)
    }

    func testRefreshSetsCountAndDockBadge() async {
        let client = FakeClient()
        client.unread = 4
        var dock: [String?] = []
        let model = MacUnreadNotices(client: client, uid: { "u1" }, applyDockBadge: { dock.append($0) })
        await model.refresh(force: true)
        XCTAssertEqual(model.count, 4)
        XCTAssertEqual(dock.last, "4")
        client.unread = 0
        await model.refresh(force: true)
        XCTAssertEqual(model.count, 0)
        XCTAssertEqual(dock.last, .some(nil), "0 になったらバッジを消す")
    }

    func testFailureKeepsCount() async {
        let client = FakeClient()
        client.unread = 2
        let model = MacUnreadNotices(client: client, uid: { "u1" }, applyDockBadge: { _ in })
        await model.refresh(force: true)
        client.fails = true
        await model.refresh(force: true)
        XCTAssertEqual(model.count, 2, "読めない時は前の数のまま")
    }

    func testSignedOutOrMockIsZero() async {
        let client = FakeClient()
        client.unread = 5
        var uid: String? = "u1"
        let model = MacUnreadNotices(client: client, uid: { uid }, applyDockBadge: { _ in })
        await model.refresh(force: true)
        uid = nil
        await model.refresh(force: true)
        XCTAssertEqual(model.count, 0, "サインアウトしたら消す")
        let mock = MacUnreadNotices(client: nil, uid: { "u1" }, applyDockBadge: { _ in })
        await mock.refresh(force: true)
        XCTAssertEqual(mock.count, 0)
    }

    func testThrottle() async {
        let client = FakeClient()
        let model = MacUnreadNotices(client: client, uid: { "u1" }, applyDockBadge: { _ in })
        let t0 = Date(timeIntervalSince1970: 1_000)
        await model.refresh(now: t0)
        await model.refresh(now: t0.addingTimeInterval(5))
        XCTAssertEqual(client.calls, 1, "続けて前に来ても問い合わせない")
        await model.refresh(now: t0.addingTimeInterval(MacUnreadNotices.minInterval))
        XCTAssertEqual(client.calls, 2)
        await model.refresh(force: true, now: t0.addingTimeInterval(MacUnreadNotices.minInterval + 1))
        XCTAssertEqual(client.calls, 3, "force は間隔を見ない")
    }
}
#endif
