import XCTest

/// `locationPicker`(#212)の UI テスト: モック(METAMON_MOCK=1・検索結果は固定)で
/// お店リスト(docs/dsl/examples/shops.json)を開き、検索 → 選ぶ → フォームに名前 → 保存 → 一覧に出る。
/// `TEST_RUNNER_PLACE_SHOT_DIR=<dir>` を付けて xcodebuild するとスクショを PNG でも書き出す。
final class LocationPickerUITests: XCTestCase {
    private var app: XCUIApplication!

    override func setUp() { continueAfterFailure = false }

    override func tearDown() {
        app?.terminate()
        app = nil
    }

    private func shot(_ name: String) {
        let s = app.screenshot()
        let a = XCTAttachment(screenshot: s)
        a.name = name
        a.lifetime = .keepAlways
        add(a)
        if let dir = ProcessInfo.processInfo.environment["PLACE_SHOT_DIR"], !dir.isEmpty {
            try? s.pngRepresentation.write(to: URL(fileURLWithPath: dir).appending(path: "\(name).png"))
        }
    }

    func testSearchPickShowsNameAndSaves() {
        app = XCUIApplication()
        app.launchEnvironment = ["METAMON_MOCK": "1", "METAMON_FAB_POSITION": "auto", "METAMON_AI_CONSENT": "granted", "METAMON_INITIAL_EXAMPLE": "shops"]
        app.launchArguments = ["-AppleLanguages", "(ja)", "-AppleLocale", "ja_JP", "-metamon.appLanguage", "ja"]
        app.launch()

        let add = app.buttons["お店を追加"].firstMatch
        XCTAssertTrue(add.waitForExistence(timeout: 20), "「お店を追加」が無い")
        add.tap()

        let open = app.buttons["locationPicker.open"]
        XCTAssertTrue(open.waitForExistence(timeout: 15), "場所を選ぶボタンが無い")
        shot("1-form-empty")
        open.tap()

        let search = app.textFields["locationPicker.search"]
        XCTAssertTrue(search.waitForExistence(timeout: 15), "検索欄が無い")
        XCTAssertTrue(app.buttons["locationPicker.current"].exists, "現在地ボタンが無い")
        XCTAssertTrue(app.buttons["locationPicker.usePin"].exists, "ピンで決めるボタンが無い")
        shot("2-sheet-map")
        search.tap()
        search.typeText("兼六")

        let first = app.buttons["locationPicker.result.0"]
        XCTAssertTrue(first.waitForExistence(timeout: 15), "検索結果が出ない")
        shot("3-sheet-results")
        first.tap()

        let value = app.staticTexts["locationPicker.value"]
        XCTAssertTrue(value.waitForExistence(timeout: 15), "フォームに選んだ場所が出ない")
        XCTAssertEqual(value.label, "兼六園")
        let name = app.textFields.matching(NSPredicate(format: "placeholderValue == 'カフェ'")).firstMatch
        XCTAssertEqual(name.value as? String, "兼六園", "nameField(お店の名前)に場所の名前が入らない")
        shot("4-form-picked")

        app.buttons["保存"].firstMatch.tap()
        let row = app.descendants(matching: .any).matching(NSPredicate(format: "label BEGINSWITH %@", "兼六園")).firstMatch
        XCTAssertTrue(row.waitForExistence(timeout: 15), "保存したお店が一覧に出ない")
        sleep(2) // 地図のタイル読み込み
        shot("5-home-saved")
    }
}
