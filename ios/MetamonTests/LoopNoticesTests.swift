#if !APP_STORE_REVIEW
import XCTest
import UserNotifications
import MetamonServices
@testable import Metamon

/// #374: 設定の「お知らせ」(一覧・未読・既読・push の同意)と、フィードバックの「直ったら知らせてほしい」。
@MainActor
final class LoopNoticesTests: XCTestCase {
    private final class FakeClient: LoopNoticesClient, @unchecked Sendable {
        var pages: [String?: LoopNoticePage] = [:]
        var listError: Error?
        var markError: Error?
        var consentError: Error?
        var consent = false
        var marked: [String] = []
        var consentSets: [Bool] = []
        var listCalls = 0

        func notices(pageToken: String?) async throws -> LoopNoticePage {
            listCalls += 1
            if let listError { throw listError }
            return pages[pageToken] ?? LoopNoticePage(items: [], unreadCount: 0)
        }

        func markRead(id: String) async throws {
            marked.append(id)
            if let markError { throw markError }
        }

        func feedbackFixedConsent() async throws -> Bool {
            if let consentError { throw consentError }
            return consent
        }

        func setFeedbackFixedConsent(_ enabled: Bool) async throws {
            consentSets.append(enabled)
            if let consentError { throw consentError }
            consent = enabled
        }
    }

    private struct Boom: Error {}

    private func notice(_ id: String, unread: Bool = true, day: Int = 1) -> LoopNotice {
        LoopNotice(id: id, title: "直りました: \(id)", body: "あなたの声で直りました",
                   createdAt: Date(timeIntervalSince1970: TimeInterval(day) * 86_400), readAt: unread ? nil : Date(timeIntervalSince1970: 0))
    }

    // MARK: 一覧

    func testLoadShowsNewestFirstWithUnreadCount() async {
        let c = FakeClient()
        c.pages[nil] = LoopNoticePage(items: [notice("b", day: 2), notice("a", unread: false)], unreadCount: 1)
        let m = NoticesModel(client: c)
        XCTAssertEqual(m.phase, .loading, "読み込み中はスケルトン")
        await m.load()
        XCTAssertEqual(m.phase, .loaded)
        XCTAssertEqual(m.items.map(\.id), ["b", "a"])
        XCTAssertEqual(m.unreadCount, 1)
        XCTAssertTrue(m.items[0].isUnread)
        XCTAssertFalse(m.items[1].isUnread)
    }

    func testEmptyAndFailure() async {
        let c = FakeClient()
        let m = NoticesModel(client: c)
        await m.load()
        XCTAssertEqual(m.phase, .loaded)
        XCTAssertTrue(m.items.isEmpty, "空の案内")

        let f = FakeClient()
        f.listError = Boom()
        let failed = NoticesModel(client: f)
        await failed.load()
        XCTAssertEqual(failed.phase, .failed)
    }

    func testRefreshFailureKeepsList() async {
        let c = FakeClient()
        c.pages[nil] = LoopNoticePage(items: [notice("a")], unreadCount: 1)
        let m = NoticesModel(client: c)
        await m.load()
        c.listError = Boom()
        await m.load() // 引っぱって更新が失敗
        XCTAssertEqual(m.phase, .loaded)
        XCTAssertEqual(m.items.map(\.id), ["a"])
    }

    func testLoadMoreAppendsNextPage() async {
        let c = FakeClient()
        c.pages[nil] = LoopNoticePage(items: [notice("c", day: 3), notice("b", day: 2)], unreadCount: 3, nextPageToken: "t2")
        c.pages["t2"] = LoopNoticePage(items: [notice("b", day: 2), notice("a")], unreadCount: 3)
        let m = NoticesModel(client: c)
        await m.load()
        await m.loadMore()
        XCTAssertEqual(m.items.map(\.id), ["c", "b", "a"], "重なりは 1 回だけ")
        XCTAssertNil(m.nextPageToken)
        let calls = c.listCalls
        await m.loadMore()
        XCTAssertEqual(c.listCalls, calls, "最後のページの後は読まない")
    }

    // MARK: 既読

    func testMarkReadIsOptimisticAndRollsBack() async {
        let c = FakeClient()
        c.pages[nil] = LoopNoticePage(items: [notice("a"), notice("b")], unreadCount: 2)
        let m = NoticesModel(client: c)
        await m.load()
        await m.markRead("a")
        XCTAssertFalse(m.items[0].isUnread)
        XCTAssertEqual(m.unreadCount, 1)
        XCTAssertEqual(c.marked, ["a"])
        await m.markRead("a")
        XCTAssertEqual(c.marked, ["a"], "既読は送り直さない")

        c.markError = Boom()
        await m.markRead("b")
        XCTAssertTrue(m.items[1].isUnread, "失敗したら戻す")
        XCTAssertEqual(m.unreadCount, 1)
    }

    func testMarkAllSeenClearsBadge() async {
        let c = FakeClient()
        c.pages[nil] = LoopNoticePage(items: [notice("a"), notice("b"), notice("c", unread: false)], unreadCount: 2)
        let m = NoticesModel(client: c)
        await m.load()
        await m.markAllSeen()
        XCTAssertEqual(Set(c.marked), ["a", "b"])
        XCTAssertEqual(m.unreadCount, 0)
    }

    // MARK: push の同意(お知らせの画面)

    func testPushToggleRollsBackOnFailure() async {
        let c = FakeClient()
        c.consent = true
        let m = NoticesModel(client: c)
        await m.loadConsent()
        XCTAssertEqual(m.pushEnabled, true)
        await m.setPush(false)
        XCTAssertEqual(m.pushEnabled, false)
        XCTAssertEqual(c.consentSets, [false])

        c.consentError = Boom()
        await m.setPush(true)
        XCTAssertEqual(m.pushEnabled, false, "失敗したら戻す")
        XCTAssertTrue(m.pushFailed)
    }

    func testConsentUnreadableHidesToggle() async {
        let c = FakeClient()
        c.consentError = Boom()
        let m = NoticesModel(client: c)
        await m.loadConsent()
        XCTAssertNil(m.pushEnabled)
        await m.setPush(true)
        XCTAssertTrue(c.consentSets.isEmpty, "読めない時は変えない")
    }

    // MARK: フィードバックの「直ったら知らせてほしい」

    private final class Calls { var requested = 0; var registered = 0 }

    private func optIn(_ c: FakeClient, status: UNAuthorizationStatus, grant: Bool = true) -> (FeedbackFixedOptIn, Calls) {
        let calls = Calls()
        let o = FeedbackFixedOptIn(client: c, authorization: { status },
                                   requestAuthorization: { calls.requested += 1; return grant },
                                   registerPush: { calls.registered += 1 })
        return (o, calls)
    }

    func testPermissionAskedOnlyWhenNotDetermined() async {
        let c = FakeClient()
        var (o, calls) = optIn(c, status: .notDetermined)
        let allowed = await o.requestPermissionIfNeeded()
        XCTAssertTrue(allowed)
        XCTAssertEqual(calls.requested, 1)
        XCTAssertEqual(calls.registered, 1, "許可されたら既存の push の登録を通す")

        (o, calls) = optIn(c, status: .authorized)
        _ = await o.requestPermissionIfNeeded()
        XCTAssertEqual(calls.requested, 0, "許可済みなら二重に聞かない")
        XCTAssertEqual(calls.registered, 1)

        (o, calls) = optIn(c, status: .denied)
        let denied = await o.requestPermissionIfNeeded()
        XCTAssertFalse(denied)
        XCTAssertEqual(calls.requested, 0, "拒否済みなら聞かない")
        XCTAssertEqual(calls.registered, 0)
        XCTAssertTrue(c.consentSets.isEmpty, "許可を聞くだけでは同意を送らない(送った後に送る)")
    }

    func testConsentSavedAfterSending() async {
        let c = FakeClient()
        let (o, _) = optIn(c, status: .authorized)
        let note = await FeedbackComposerView.fixedNoticeNote(o)
        XCTAssertNil(note)
        XCTAssertEqual(c.consentSets, [true])
        XCTAssertTrue(c.consent)
    }

    func testConsentNotesWhenDeniedOrFailed() async {
        let denied = FakeClient()
        let (o, _) = optIn(denied, status: .denied)
        let note = await FeedbackComposerView.fixedNoticeNote(o)
        XCTAssertNotNil(note, "通知が届かないことを伝える")
        XCTAssertEqual(denied.consentSets, [true], "同意は保存する(後で許可すれば届く)")

        let failing = FakeClient()
        failing.consentError = Boom()
        let (f, _) = optIn(failing, status: .authorized)
        let failedNote = await FeedbackComposerView.fixedNoticeNote(f)
        XCTAssertNotNil(failedNote)
    }

    // MARK: 契約

    func testDecodesPage() throws {
        let json = #"{"items":[{"id":"ntc_0123456789abcdef01234567","title":"直りました","body":"本文","createdAt":"2026-11-10T03:00:00.123Z","readAt":"2026-11-10T04:00:00Z"},{"id":"ntc_1","title":"t","body":"b","createdAt":"2026-11-09T03:00:00Z"}],"unreadCount":1,"nextPageToken":"NTA"}"#
        let page = try JSONDecoder().decode(LoopNoticePage.self, from: Data(json.utf8))
        XCTAssertEqual(page.items.count, 2)
        XCTAssertEqual(page.unreadCount, 1)
        XCTAssertEqual(page.nextPageToken, "NTA")
        XCTAssertFalse(page.items[0].isUnread)
        XCTAssertTrue(page.items[1].isUnread)
        XCTAssertEqual(page.items[0].createdAt.timeIntervalSince1970, 1_794_279_600.123, accuracy: 0.01)
    }
}
#endif
