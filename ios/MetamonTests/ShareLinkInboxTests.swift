#if !APP_STORE_REVIEW
import XCTest
@testable import Metamon

/// #278: インストール直後(サインイン画面)に届いたシェアリンクを保留し、bootstrap 後に 1 回だけ開く。
@MainActor
final class ShareLinkInboxTests: XCTestCase {
    private let link = URL(string: "tailorit://import/tok_new")!

    func testHoldsLinkUntilReady() {
        let ui = SharingUI()
        XCTAssertTrue(ui.receive(link))
        XCTAssertNil(ui.takeLink(), "サインイン前・bootstrap 前は開かない")
        XCTAssertEqual(ui.pendingLink, link, "保留は残す")
        ui.isReady = true
        XCTAssertEqual(ui.takeLink(), link)
        XCTAssertNil(ui.takeLink(), "開くのは 1 回だけ")
    }

    func testIgnoresOtherURLs() {
        let ui = SharingUI()
        ui.isReady = true
        XCTAssertFalse(ui.receive(URL(string: "metamon://widget")!))
        XCTAssertFalse(ui.receive(URL(string: "https://example.com/s/tok")!))
        XCTAssertFalse(ui.receive(URL(string: "tailorit://import/")!))
        XCTAssertNil(ui.takeLink())
    }

    /// #331: Universal Links は Web 閲覧の NSUserActivity で届く。シェアリンクの URL だけ保留する
    func testReceivesUniversalLinkActivity() {
        let ui = SharingUI()
        let universal = URL(string: "https://tailor-it.app/s/tok_web")!
        let activity = NSUserActivity(activityType: NSUserActivityTypeBrowsingWeb)
        activity.webpageURL = universal
        XCTAssertTrue(ui.receive(activity: activity))
        ui.isReady = true
        XCTAssertEqual(ui.takeLink(), universal)

        let other = NSUserActivity(activityType: NSUserActivityTypeBrowsingWeb)
        other.webpageURL = URL(string: "https://tailor-it.app/terms")!
        XCTAssertFalse(ui.receive(activity: other))
        let notWeb = NSUserActivity(activityType: "jp.kz2601.metamon.other")
        notWeb.webpageURL = universal
        XCTAssertFalse(ui.receive(activity: notWeb))
        XCTAssertNil(ui.takeLink())
    }

    func testLatestLinkWins() {
        let ui = SharingUI()
        ui.receive(link)
        ui.receive(URL(string: "tailorit://import/tok_second")!)
        ui.isReady = true
        XCTAssertEqual(ui.takeLink()?.absoluteString, "tailorit://import/tok_second")
    }

    func testSignOutWaitsForNextBootstrap() {
        let ui = SharingUI()
        ui.isReady = true
        ui.isReady = false // RootView が消えた(サインアウト)
        ui.receive(link)
        XCTAssertNil(ui.takeLink())
        ui.isReady = true
        XCTAssertEqual(ui.takeLink(), link)
    }
}
#endif
