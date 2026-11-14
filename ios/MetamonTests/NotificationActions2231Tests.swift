import XCTest
import UserNotifications
@testable import Metamon

/// 通知のボタンと返信の土台(#2231): カテゴリの登録内容と、応答を `NotificationReply` に変える所。
@MainActor
final class NotificationActions2231Tests: XCTestCase {
    func testCategoriesHaveOpenAndTestActions() throws {
        let all = NotificationCategories.all
        XCTAssertEqual(Set(all.map(\.identifier)), [NotificationCategoryID.open, NotificationCategoryID.testActions])
        let open = try XCTUnwrap(all.first { $0.identifier == NotificationCategoryID.open })
        XCTAssertTrue(open.actions.isEmpty)
        let test = try XCTUnwrap(all.first { $0.identifier == NotificationCategoryID.testActions })
        XCTAssertEqual(test.actions.map(\.identifier), [NotificationActionID.record, NotificationActionID.reply])
        XCTAssertTrue(test.actions[1] is UNTextInputNotificationAction)
        XCTAssertFalse(test.actions[0] is UNTextInputNotificationAction)
    }

    func testButtonResponse() {
        let r = NotificationReply.make(actionIdentifier: NotificationActionID.record, categoryIdentifier: NotificationCategoryID.testActions,
                                       notificationId: "n1", userInfo: ["appId": "app_1", "n": 3], userText: nil)
        XCTAssertEqual(r.action, .record)
        XCTAssertNil(r.text)
        XCTAssertEqual(r.appId, "app_1")
        XCTAssertEqual(r.info, ["appId": "app_1"]) // 文字列でない値は落とす
        XCTAssertEqual(r.categoryId, NotificationCategoryID.testActions)
        XCTAssertEqual(r.notificationId, "n1")
        XCTAssertFalse(r.isPlainOpen)
    }

    func testTextInputResponse() {
        let r = NotificationReply.make(actionIdentifier: NotificationActionID.reply, categoryIdentifier: NotificationCategoryID.testActions,
                                       notificationId: "n2", userInfo: ["appId": "app_2"], userText: "  牛乳を買った \n")
        XCTAssertEqual(r.action, .reply)
        XCTAssertEqual(r.text, "牛乳を買った")
        XCTAssertFalse(r.description.contains("牛乳"), "打った文字はログに出さない")
        let blank = NotificationReply.make(actionIdentifier: NotificationActionID.reply, categoryIdentifier: "c",
                                           notificationId: "n3", userInfo: [:], userText: "   ")
        XCTAssertNil(blank.text)
    }

    func testUnknownAndSystemIdentifiers() {
        XCTAssertEqual(NotificationReply.action(for: "metamon.action.removedInOldVersion"), .unknown("metamon.action.removedInOldVersion"))
        XCTAssertEqual(NotificationReply.action(for: UNNotificationDefaultActionIdentifier), .open)
        XCTAssertEqual(NotificationReply.action(for: UNNotificationDismissActionIdentifier), .dismiss)
    }

    func testBadAppIdIsDropped() {
        let r = NotificationReply.make(actionIdentifier: NotificationActionID.record, categoryIdentifier: "c", notificationId: "n",
                                       userInfo: ["appId": "../etc passwd"], userText: nil)
        XCTAssertNil(r.appId)
        XCTAssertEqual(r.info["appId"], "../etc passwd")
    }

    func testInboxKeepsPendingUntilInstalledAndSkipsPlainOpen() {
        let inbox = NotificationReplyInbox()
        let open = NotificationReply.make(actionIdentifier: UNNotificationDefaultActionIdentifier, categoryIdentifier: "c",
                                          notificationId: "n", userInfo: [:], userText: nil)
        let record = NotificationReply.make(actionIdentifier: NotificationActionID.record, categoryIdentifier: "c",
                                            notificationId: "n", userInfo: [:], userText: nil)
        inbox.receive(open)
        XCTAssertNil(inbox.pending)
        inbox.receive(record)
        XCTAssertEqual(inbox.pending, record)
        var got: [NotificationReply] = []
        inbox.install { got.append($0) }
        XCTAssertEqual(got, [record])
        XCTAssertNil(inbox.pending)
        inbox.receive(record)
        XCTAssertEqual(got.count, 2)
    }

    func testPresenterStillHandlesTheTap() {
        let selector = #selector(UNUserNotificationCenterDelegate.userNotificationCenter(_:didReceive:withCompletionHandler:))
        XCTAssertTrue(ForegroundNotificationPresenter.shared.responds(to: selector))
    }
}
