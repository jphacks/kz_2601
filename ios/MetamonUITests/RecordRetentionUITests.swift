import XCTest

/// 記録の保存期間(#482)の UI テスト: モックで retentionDays 3 の日記を開くと確認ダイアログが出て、
/// 「自動で消す」で 3 日より前に作った記録だけが一覧から消える。
/// seed は `METAMON_SEED_DUMMY=6` + `METAMON_SEED_STEP_HOURS=24`(1 件ずつ 1 日古い・DEBUG のみ)。
/// 確認は appId ごとに端末へ残るので、DSL は毎回別名のファイルに書く(appId = "visual-<ファイル名>")。
final class RecordRetentionUITests: XCTestCase {
    private var app: XCUIApplication!
    private var dslURL: URL?

    override func setUp() { continueAfterFailure = false }

    override func tearDown() {
        app?.terminate()
        app = nil
        if let dslURL { try? FileManager.default.removeItem(at: dslURL) }
    }

    private static let dsl = """
    {"schemaVersion":1,"app":{"name":"日記","voiceEntry":"floating"},"initialScreen":"home",
     "entities":[{"id":"diary","name":"日記","titleField":"title","retentionDays":3,
                  "fields":[{"id":"title","name":"タイトル","type":"string"}]}],
     "screens":[{"id":"home","title":"日記","components":[{"type":"list","entity":"diary","area":"A1:F12"}]}]}
    """

    func testConfirmDeletesOnlyExpiredRecords() throws {
        let url = FileManager.default.temporaryDirectory.appending(path: "retention-\(UUID().uuidString.prefix(8)).json")
        try Data(Self.dsl.utf8).write(to: url)
        dslURL = url

        app = XCUIApplication()
        app.launchEnvironment = [
            "METAMON_MOCK": "1", "METAMON_FAB_POSITION": "auto", "METAMON_AI_CONSENT": "granted",
            "METAMON_INITIAL_DSL_PATH": url.path, "METAMON_SEED_DUMMY": "6", "METAMON_SEED_STEP_HOURS": "24",
        ]
        app.launchArguments = ["-AppleLanguages", "(ja)", "-AppleLocale", "ja_JP", "-metamon.appLanguage", "ja"]
        app.launch()

        let alert = app.alerts["古い記録を自動で消しますか?"]
        XCTAssertTrue(alert.waitForExistence(timeout: 20), "保存期間の確認が出ない")
        XCTAssertTrue(alert.staticTexts.containing(NSPredicate(format: "label CONTAINS %@", "「日記」は3日たった記録を自動で消します")).firstMatch.exists)
        let oldest = app.staticTexts["タイトル 6"]
        XCTAssertTrue(oldest.exists || oldest.waitForExistence(timeout: 5), "確認前は古い記録も残っている")
        add(XCTAttachment(screenshot: app.screenshot()))

        alert.buttons["自動で消す"].tap()

        // 0〜2 日前は残り、3 日以上前(4〜6 件目)は消える
        let gone = NSPredicate(format: "exists == false")
        expectation(for: gone, evaluatedWith: oldest)
        waitForExpectations(timeout: 10)
        XCTAssertFalse(app.staticTexts["タイトル 5"].exists)
        XCTAssertFalse(app.staticTexts["タイトル 4"].exists)
        XCTAssertTrue(app.staticTexts["タイトル 1"].exists)
        XCTAssertTrue(app.staticTexts["タイトル 2"].exists)
        let shot = XCTAttachment(screenshot: app.screenshot())
        shot.lifetime = .keepAlways
        add(shot)
    }
}
