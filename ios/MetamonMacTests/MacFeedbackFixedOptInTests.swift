#if !APP_STORE_REVIEW
import XCTest
import UserNotifications
import MetamonServices
@testable import Metamon

/// #364: Mac のフィードバックの「直ったら知らせてほしい」(iOS の `FeedbackFixedOptIn` と同じ動き)。
@MainActor
final class MacFeedbackFixedOptInTests: XCTestCase {
    private final class FakeClient: LoopNoticesClient, @unchecked Sendable {
        var consentError: Error?
        var consentSets: [Bool] = []

        func notices(pageToken: String?) async throws -> LoopNoticePage { LoopNoticePage(items: [], unreadCount: 0) }
        func markRead(id: String) async throws {}
        func feedbackFixedConsent() async throws -> Bool { consentSets.last ?? false }
        func setFeedbackFixedConsent(_ enabled: Bool) async throws {
            consentSets.append(enabled)
            if let consentError { throw consentError }
        }
    }

    private struct Boom: Error {}
    private final class Calls { var requested = 0; var registered = 0 }

    private func choice(_ c: FakeClient, status: UNAuthorizationStatus) -> (MacFixedNoticeChoice, Calls) {
        let calls = Calls()
        let optIn = FeedbackFixedOptIn(client: c, authorization: { status },
                                       requestAuthorization: { calls.requested += 1; return true },
                                       registerPush: { calls.registered += 1 })
        return (MacFixedNoticeChoice(optIn: optIn), calls)
    }

    func testDefaultOffSendsNothing() async {
        let c = FakeClient()
        let (choice, calls) = choice(c, status: .notDetermined)
        XCTAssertTrue(choice.isAvailable)
        XCTAssertFalse(choice.wants, "既定はオフ")
        let note = await choice.afterSent()
        XCTAssertNil(note)
        XCTAssertTrue(c.consentSets.isEmpty, "オフのままなら同意を送らない")
        XCTAssertEqual(calls.requested, 0, "オフのままなら許可を聞かない")
    }

    func testTurningOnAsksOnceAndSendsConsentAfterSending() async {
        let c = FakeClient()
        let (choice, calls) = choice(c, status: .notDetermined)
        await choice.set(true)
        XCTAssertTrue(choice.wants)
        XCTAssertEqual(calls.requested, 1, "未決定の時だけ OS の確認")
        XCTAssertEqual(calls.registered, 1, "許可されたら push の登録を通す")
        XCTAssertTrue(c.consentSets.isEmpty, "切り替えただけでは送らない(送った後に送る)")
        let note = await choice.afterSent()
        XCTAssertNil(note)
        XCTAssertEqual(c.consentSets, [true], "PUT /v1/me/notify-consent/feedback-fixed")
    }

    func testTurningOffAgainSendsNothing() async {
        let c = FakeClient()
        let (choice, _) = choice(c, status: .authorized)
        await choice.set(true)
        await choice.set(false)
        _ = await choice.afterSent()
        XCTAssertTrue(c.consentSets.isEmpty)
    }

    func testNotesWhenDeniedOrNotSaved() async {
        let denied = FakeClient()
        let (d, calls) = choice(denied, status: .denied)
        await d.set(true)
        XCTAssertEqual(calls.requested, 0, "拒否済みなら聞かない")
        let deniedNote = await d.afterSent()
        XCTAssertNotNil(deniedNote, "通知が届かないことを伝える")
        XCTAssertEqual(denied.consentSets, [true], "同意は保存する(後で許可すれば届く)")

        let failing = FakeClient()
        failing.consentError = Boom()
        let (f, _) = choice(failing, status: .authorized)
        await f.set(true)
        let failedNote = await f.afterSent()
        XCTAssertNotNil(failedNote)
    }

    func testMockHidesToggle() async {
        let choice = MacFixedNoticeChoice(optIn: nil)
        XCTAssertFalse(choice.isAvailable)
        await choice.set(true)
        let note = await choice.afterSent()
        XCTAssertNil(note)
    }
}
#endif
