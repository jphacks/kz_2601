#if !APP_STORE_REVIEW
import XCTest
import MetamonDSL
@testable import MetamonRenderer
import MetamonServices
@testable import Metamon

/// 連係の Handoff(#1851): 今の画面から activity を作る・届いた activity を読んでアプリと画面を開く。家計簿の例で確かめる
@MainActor
final class MacHandoffTests: XCTestCase {
    private func kakeibo() throws -> AppDSL {
        let url = try XCTUnwrap(Bundle(for: MacAppRuntime.self).url(forResource: "kakeibo", withExtension: "json"))
        return try AppDSL.load(contentsOf: url)
    }

    private func runtime() throws -> MacAppRuntime {
        MacAppRuntime(appId: "test-handoff-\(UUID().uuidString.lowercased())", dsl: try kakeibo(),
                      ai: MockAIClient(delay: .zero), isAIConsentGranted: { false })
    }

    // MARK: - 出す

    func testTargetIsTheTopScreenAndItsRecord() throws {
        let rt = try runtime()
        let home = HandoffTarget(appId: "app1", navigation: rt.navigationState, dsl: rt.dsl)
        XCTAssertEqual(home.screenId, NavigationResolver.top(rt.navigationState, dsl: rt.dsl).screen)
        XCTAssertNil(home.recordId)

        rt.reveal(screen: "expense_form", recordId: "r1")
        XCTAssertEqual(HandoffTarget(appId: "app1", navigation: rt.navigationState, dsl: rt.dsl),
                       HandoffTarget(appId: "app1", screenId: "expense_form", recordId: "r1"))
    }

    func testActivityCarriesOnlyIdsAndAppName() {
        let activity = NSUserActivity(activityType: HandoffActivity.type)
        HandoffActivity.configure(activity, target: HandoffTarget(appId: "app1", screenId: "expense_form", recordId: "r1"),
                                  appName: "家計簿")
        XCTAssertEqual(activity.title, "家計簿")
        XCTAssertEqual(activity.userInfo as? [String: String],
                       ["appId": "app1", "screenId": "expense_form", "recordId": "r1"])
        XCTAssertEqual(activity.requiredUserInfoKeys, ["appId"])
        XCTAssertNil(activity.webpageURL)
        XCTAssertTrue(activity.isEligibleForHandoff)
        XCTAssertFalse(activity.isEligibleForSearch, "記録の画面を Spotlight に出さない")
        XCTAssertFalse(activity.isEligibleForPublicIndexing)
    }

    /// 記録の無い画面に移ったら、前の記録の id を残さない
    func testReconfiguringDropsThePreviousRecord() {
        let activity = NSUserActivity(activityType: HandoffActivity.type)
        HandoffActivity.configure(activity, target: HandoffTarget(appId: "app1", screenId: "expense_form", recordId: "r1"), appName: "家計簿")
        HandoffActivity.configure(activity, target: HandoffTarget(appId: "app1", screenId: "home"), appName: "家計簿")
        XCTAssertEqual(activity.userInfo as? [String: String], ["appId": "app1", "screenId": "home"])
    }

    func testWebpageURLIsOnlyHttps() {
        let activity = NSUserActivity(activityType: HandoffActivity.type)
        let target = HandoffTarget(appId: "app1")
        HandoffActivity.configure(activity, target: target, appName: "家計簿", webpageURL: URL(string: "https://tailor-it.app/s/tok_1"))
        XCTAssertEqual(activity.webpageURL?.absoluteString, "https://tailor-it.app/s/tok_1")
        HandoffActivity.configure(activity, target: target, appName: "家計簿", webpageURL: URL(string: "tailorit://import/tok_1"))
        XCTAssertNil(activity.webpageURL, "webpageURL は http(s) だけ")
    }

    // MARK: - 受ける

    func testReadsTheActivityBack() {
        let activity = NSUserActivity(activityType: HandoffActivity.type)
        let target = HandoffTarget(appId: "app1", screenId: "expense_form", recordId: "r1")
        HandoffActivity.configure(activity, target: target, appName: "家計簿")
        XCTAssertEqual(HandoffTarget(activity: activity), target)
    }

    func testIgnoresOtherActivities() {
        let web = NSUserActivity(activityType: NSUserActivityTypeBrowsingWeb)
        web.userInfo = ["appId": "app1"]
        XCTAssertNil(HandoffTarget(activity: web))
        let noApp = NSUserActivity(activityType: HandoffActivity.type)
        noApp.userInfo = ["screenId": "home"]
        XCTAssertNil(HandoffTarget(activity: noApp), "appId が無ければ開かない")
        XCTAssertNil(HandoffTarget(userInfo: ["appId": ""]))
    }

    func testInboxOpensTheAppAndKeepsTheScreenForIt() {
        var opened: [String] = []
        let inbox = MacHandoffInbox(openApp: { opened.append($0) })
        let activity = NSUserActivity(activityType: HandoffActivity.type)
        HandoffActivity.configure(activity, target: HandoffTarget(appId: "app1", screenId: "expense_form", recordId: "r1"), appName: "家計簿")
        let now = Date(timeIntervalSince1970: 1_000)

        XCTAssertTrue(inbox.receive(activity, now: now))
        XCTAssertEqual(opened, ["app1"], "押された通知と同じ道でアプリを開く")
        XCTAssertNil(inbox.take(appId: "other", now: now), "別のアプリのキャンバスには渡さない")
        XCTAssertEqual(inbox.take(appId: "app1", now: now), HandoffTarget(appId: "app1", screenId: "expense_form", recordId: "r1"))
        XCTAssertNil(inbox.take(appId: "app1", now: now), "1 回だけ")

        XCTAssertFalse(inbox.receive(NSUserActivity(activityType: NSUserActivityTypeBrowsingWeb), now: now))
        XCTAssertEqual(opened, ["app1"], "Handoff でない activity は触らない")
    }

    func testStaleHandoffIsDropped() {
        var pending = PendingHandoff()
        let now = Date(timeIntervalSince1970: 1_000)
        pending.receive(HandoffTarget(appId: "app1", screenId: "home"), now: now)
        XCTAssertNil(pending.take(appId: "app1", now: now.addingTimeInterval(PendingHandoff.lifetime + 1)),
                     "開けなかったアプリを後で手で開いた時に勝手に画面を動かさない")
        XCTAssertNil(pending.target)
    }

    func testRevealOpensTheHandedOffScreen() throws {
        let rt = try runtime()
        rt.reveal(HandoffTarget(appId: rt.appId, screenId: "expense_form", recordId: "r1"))
        let top = NavigationResolver.top(rt.navigationState, dsl: rt.dsl)
        XCTAssertEqual(top.screen, "expense_form")
        XCTAssertEqual(top.recordId, "r1")

        let before = rt.navigationState
        rt.reveal(HandoffTarget(appId: rt.appId, screenId: "no_such_screen"))
        XCTAssertEqual(rt.navigationState, before, "無い画面は動かない")
        rt.reveal(HandoffTarget(appId: rt.appId))
        XCTAssertEqual(rt.navigationState, before, "画面の無い Handoff はアプリを開くだけ")
    }
}
#endif
