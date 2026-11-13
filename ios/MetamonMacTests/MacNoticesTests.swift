#if !APP_STORE_REVIEW
import XCTest
import MetamonServices
@testable import Metamon

/// #364: Mac の設定の「お知らせ」(一覧・既読・サインアウトで消える)。
@MainActor
final class MacNoticesTests: XCTestCase {
    private final class FakeClient: LoopNoticesClient, @unchecked Sendable {
        var page = LoopNoticePage(items: [], unreadCount: 0)
        var marked: [String] = []

        func notices(pageToken: String?) async throws -> LoopNoticePage { page }
        func markRead(id: String) async throws { marked.append(id) }
        func feedbackFixedConsent() async throws -> Bool { false }
        func setFeedbackFixedConsent(_ enabled: Bool) async throws {}
    }

    private func notice(_ id: String, unread: Bool = true) -> LoopNotice {
        LoopNotice(id: id, title: "直りました: \(id)", body: "あなたの声で直りました",
                   createdAt: Date(timeIntervalSince1970: 86_400), readAt: unread ? nil : Date(timeIntervalSince1970: 0))
    }

    func testListAndMarkRead() async throws {
        let c = FakeClient()
        c.page = LoopNoticePage(items: [notice("b"), notice("a", unread: false)], unreadCount: 1)
        let store = MacNoticesStore(client: c)
        let m = try XCTUnwrap(store.model(for: "u1"))
        XCTAssertEqual(m.phase, .loading, "読み込み中はスケルトン")
        await m.load()
        XCTAssertEqual(m.items.map(\.id), ["b", "a"])
        XCTAssertEqual(m.unreadCount, 1)

        await m.markRead("b")
        XCTAssertEqual(c.marked, ["b"], "開いたら POST /v1/me/notices/{id}/read")
        XCTAssertFalse(m.items[0].isUnread)
        XCTAssertEqual(m.unreadCount, 0)
    }

    func testSameAccountKeepsListAndSignOutDropsIt() async throws {
        let c = FakeClient()
        c.page = LoopNoticePage(items: [notice("a")], unreadCount: 1)
        let store = MacNoticesStore(client: c)
        let m = try XCTUnwrap(store.model(for: "u1"))
        await m.load()
        XCTAssertTrue(store.model(for: "u1") === m, "同じアカウントなら読み直さない")

        XCTAssertNil(store.model(for: nil), "サインアウトで消える")
        let next = try XCTUnwrap(store.model(for: "u1"))
        XCTAssertFalse(next === m)
        XCTAssertTrue(next.items.isEmpty, "前の一覧を出さない")

        let other = try XCTUnwrap(store.model(for: "u2"))
        XCTAssertFalse(other === next, "別のアカウントは別の一覧")
    }

    func testMockHasNoNotices() {
        XCTAssertNil(MacNoticesStore(client: nil).model(for: "u1"), "サーバーにつながっていない時は出さない")
    }
}
#endif
