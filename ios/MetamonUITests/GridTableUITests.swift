import XCTest

/// `gridTable`(#234)の UI テスト: モック(METAMON_MOCK=1・時間割の seed 入り)で時間割(docs/dsl/examples/timetable.json)を開き、
/// 空きマス(火 2 限)をタップ → 曜日・時限が入ったフォーム → 科目を入れて保存 → マスに出る。
/// `GRID_SHOT_DIR=<dir>`(xcodebuild では `TEST_RUNNER_GRID_SHOT_DIR`)でスクショを PNG でも書き出す。
final class GridTableUITests: XCTestCase {
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
        let env = ProcessInfo.processInfo.environment
        if let dir = env["GRID_SHOT_DIR"], !dir.isEmpty {
            let suffix = env["GRID_SHOT_SUFFIX"] ?? ""
            try? s.pngRepresentation.write(to: URL(fileURLWithPath: dir).appending(path: "\(name)\(suffix).png"))
        }
    }

    func testEmptyCellOpensPrefilledFormAndSaves() {
        app = XCUIApplication()
        app.launchEnvironment = ["METAMON_MOCK": "1", "METAMON_FAB_POSITION": "auto", "METAMON_AI_CONSENT": "granted", "METAMON_INITIAL_EXAMPLE": "timetable"]
        app.launchArguments = ["-AppleLanguages", "(ja)", "-AppleLocale", "ja_JP", "-metamon.appLanguage", "ja"]
        app.launch()

        // seed: 月1 線形代数 / 水2 英語 + 英語演習(+1)/ 金3 プログラミング
        let seeded = app.buttons["gridTable.cell.0.0"]
        XCTAssertTrue(seeded.waitForExistence(timeout: 20), "時間割のマスが無い")
        XCTAssertTrue((seeded.value as? String ?? "").contains("線形代数"), "seed の授業がマスに無い: \(seeded.value ?? "nil")")
        shot("1-home")

        let empty = app.buttons["gridTable.cell.1.1"] // 火 2 限
        XCTAssertEqual(empty.value as? String, "空き")
        empty.tap()

        let subject = app.textFields.matching(NSPredicate(format: "placeholderValue == '線形代数'")).firstMatch
        XCTAssertTrue(subject.waitForExistence(timeout: 15), "追加フォームが開かない")
        XCTAssertTrue(app.buttons["火"].firstMatch.isSelected, "曜日に「火」が入っていない")
        let period = app.staticTexts.matching(NSPredicate(format: "label MATCHES %@", "^2 ?限$")).firstMatch
        XCTAssertTrue(period.exists, "時限に 2 が入っていない")
        shot("2-form-prefilled")

        subject.tap()
        subject.typeText("統計学")
        app.buttons["保存"].firstMatch.tap()

        let saved = app.buttons["gridTable.cell.1.1"]
        let done = NSPredicate(format: "value CONTAINS %@", "統計学")
        expectation(for: done, evaluatedWith: saved)
        waitForExpectations(timeout: 15)
        shot("3-home-saved")
    }
}
