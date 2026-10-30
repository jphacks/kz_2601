import XCTest

/// #253: 公開の記録に個人の情報が入る生成の確認。モック(`METAMON_MOCK=1`)の「予約」は、お名前・電話番号が
/// 誰でも見られる予約アプリと `meta.publicPersonalData` を返す。
final class PublicDataUITests: XCTestCase {
    private var app: XCUIApplication!

    override func setUp() {
        continueAfterFailure = false
    }

    override func tearDown() {
        app?.terminate()
        app = nil
    }

    private func launch(clarify: Bool, linkPrompted: Bool = true) {
        app = XCUIApplication()
        app.launchEnvironment = ["METAMON_MOCK": "1", "METAMON_FAB_POSITION": "auto", "METAMON_AI_CONSENT": "granted",
                                 "METAMON_CLARIFY": clarify ? "1" : "0"]
        app.launchArguments = ["-AppleLanguages", "(ja)", "-AppleLocale", "ja_JP", "-metamon.appLanguage", "ja",
                               "-metamon.accountLinkPrompted", linkPrompted ? "YES" : "NO"]
        app.launch()
    }

    private func say(_ text: String) {
        let field = app.descendants(matching: .any)["composer.field"].firstMatch
        XCTAssertTrue(field.waitForExistence(timeout: 15), "初期画面の文字入力が無い")
        field.tap()
        field.typeText(text)
        app.buttons["送信"].firstMatch.tap()
    }

    /// 生成直後にダイアログ →「公開のままにする」→ 一覧とフォームに「誰でも見られます」
    func testDialogAfterGeneratingPublicPersonalData() {
        launch(clarify: false)
        say("美容室の予約アプリを作って")

        let alert = app.alerts["個人の情報も公開されます"]
        XCTAssertTrue(alert.waitForExistence(timeout: 30), "meta.publicPersonalData があるのにダイアログが出ない")
        XCTAssertTrue(alert.staticTexts["お名前・電話番号は、アプリを開いた人なら誰でも見られます。"].exists)
        XCTAssertTrue(alert.buttons["この欄を消す"].exists)
        XCTAssertTrue(alert.buttons["自分だけにする"].exists)
        attach("pii-dialog")

        alert.buttons["公開のままにする"].tap()
        XCTAssertTrue(waitGone(alert))
        let notice = app.descendants(matching: .any)["publicDataNotice"].firstMatch
        XCTAssertTrue(notice.waitForExistence(timeout: 10), "公開の一覧に「誰でも見られます」が無い")
        let banner = app.staticTexts.containing(NSPredicate(format: "label CONTAINS %@", "ほかの人に見られてよいか")).firstMatch
        if banner.exists { banner.tap() } // 上の要約カード(タップで消える)が一覧の見出しに被らないように
        _ = waitGone(banner, timeout: 10)
        attach("pii-list-notice")

        app.buttons["予約を追加"].firstMatch.tap()
        XCTAssertTrue(app.textFields["お名前"].firstMatch.waitForExistence(timeout: 10) || app.staticTexts["お名前"].firstMatch.waitForExistence(timeout: 5))
        XCTAssertTrue(app.descendants(matching: .any)["publicDataNotice"].firstMatch.exists, "公開のフォームに「誰でも見られます」が無い")
        attach("pii-form-notice")
    }

    /// 初めて作った後の「引き継ぎのためにサインイン」の案内は、このダイアログに答えてから出る(先に出るとダイアログが出ない)
    func testLinkPromptWaitsForDialog() {
        launch(clarify: false, linkPrompted: false)
        say("美容室の予約アプリを作って")

        let alert = app.alerts["個人の情報も公開されます"]
        XCTAssertTrue(alert.waitForExistence(timeout: 30), "引き継ぎの案内が先に出てダイアログが出ない")
        let link = app.staticTexts["作ったアプリを残しておきませんか"].firstMatch
        XCTAssertFalse(link.waitForExistence(timeout: 5), "ダイアログに答える前に引き継ぎの案内が出た")
        XCTAssertTrue(alert.exists, "引き継ぎの案内でダイアログが消えた")

        alert.buttons["公開のままにする"].tap()
        XCTAssertTrue(link.waitForExistence(timeout: 10), "ダイアログに答えた後に引き継ぎの案内が出ない")
        app.buttons["あとで"].firstMatch.tap()
        XCTAssertTrue(waitGone(link))
    }

    /// 「自分だけにする」で修正が走り、公開の表示が消える
    func testMakePrivateRemovesNotice() {
        launch(clarify: false)
        say("美容室の予約アプリを作って")
        let alert = app.alerts["個人の情報も公開されます"]
        XCTAssertTrue(alert.waitForExistence(timeout: 30))
        alert.buttons["自分だけにする"].tap()
        let notice = app.descendants(matching: .any)["publicDataNotice"].firstMatch
        XCTAssertTrue(waitGone(notice, timeout: 20), "自分だけにしても公開の表示が残る")
        XCTAssertTrue(app.buttons["予約を追加"].firstMatch.waitForExistence(timeout: 10))
        XCTAssertFalse(alert.exists, "自分だけにした後の生成でダイアログが再び出た")
    }

    /// チャットの確認(confirmPublic)の吹き出しに公開の印
    func testChatConfirmBubbleIsMarked() {
        launch(clarify: true)
        say("美容室の予約アプリ。お客さんの名前と電話番号も残したい")
        XCTAssertTrue(app.staticTexts.containing(NSPredicate(format: "label CONTAINS %@", "公開の記録の確認")).firstMatch.waitForExistence(timeout: 20),
                      "confirmPublic の question に公開の印が無い")
        attach("pii-chat-confirm")
    }

    private func waitGone(_ element: XCUIElement, timeout: TimeInterval = 10) -> Bool {
        let exp = XCTNSPredicateExpectation(predicate: NSPredicate(format: "exists == false"), object: element)
        return XCTWaiter().wait(for: [exp], timeout: timeout) == .completed
    }

    private func attach(_ name: String) {
        let shot = XCTAttachment(screenshot: app.screenshot())
        shot.name = name
        shot.lifetime = .keepAlways
        add(shot)
    }
}
