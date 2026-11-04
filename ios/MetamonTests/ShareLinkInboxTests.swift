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
