import XCTest

/// `Field.hidden`(#262)の UI テスト: モックでクイズ(backend/internal/ai/examples/quiz.json)を開き、
/// 自分が作った問題のフォームには正解欄があり、別の人が作った問題のフォームには無いことを確かめる。
/// seed は `METAMON_SEED_DUMMY=2` + `METAMON_SEED_AUTHORS=mixed`(1 件目 = 自分 / 2 件目 = 別の人・DEBUG のみ)。
/// `QUIZ_SHOT_DIR=<dir>`(xcodebuild では `TEST_RUNNER_QUIZ_SHOT_DIR`)でスクショを PNG でも書き出す。
final class QuizHiddenUITests: XCTestCase {
    private var app: XCUIApplication!

    override func setUp() { continueAfterFailure = false }

    override func tearDown() {
        app?.terminate()
        app = nil
    }

    /// このファイル → MetamonUITests → ios → リポジトリ root
    private var quizPath: String {
        var url = URL(fileURLWithPath: #filePath)
        for _ in 0..<3 { url.deleteLastPathComponent() }
        return url.appending(path: "backend/internal/ai/examples/quiz.json").path
    }

    private func shot(_ name: String) {
        let s = app.screenshot()
        let a = XCTAttachment(screenshot: s)
        a.name = name
        a.lifetime = .keepAlways
        add(a)
        if let dir = ProcessInfo.processInfo.environment["QUIZ_SHOT_DIR"], !dir.isEmpty {
            try? s.pngRepresentation.write(to: URL(fileURLWithPath: dir).appending(path: "\(name).png"))
        }
    }

    func testAnswerFieldOnlyForQuestionAuthor() {
        app = XCUIApplication()
        app.launchEnvironment = [
            "METAMON_MOCK": "1", "METAMON_FAB_POSITION": "auto", "METAMON_AI_CONSENT": "granted",
            "METAMON_INITIAL_DSL_PATH": quizPath, "METAMON_SEED_DUMMY": "2", "METAMON_SEED_AUTHORS": "mixed",
        ]
        app.launchArguments = ["-AppleLanguages", "(ja)", "-AppleLocale", "ja_JP", "-metamon.appLanguage", "ja"]
        app.launch()

        let mine = app.staticTexts["問題文 1"]
        XCTAssertTrue(mine.waitForExistence(timeout: 20), "クイズの問題一覧が無い")
        XCTAssertTrue(app.staticTexts["問題文 2"].exists)
        shot("1-home")

        // 自分が作った問題: 正解欄がある
        mine.tap()
        let textLabel = app.staticTexts["問題文"]
        XCTAssertTrue(textLabel.waitForExistence(timeout: 15), "問題のフォームが開かない")
        XCTAssertTrue(app.staticTexts["正解"].exists, "作者のフォームに正解欄が無い")
        shot("2-form-author")
        app.navigationBars.buttons.element(boundBy: 0).tap()

        // 別の人が作った問題: 正解欄が無い
        let theirs = app.staticTexts["問題文 2"]
        XCTAssertTrue(theirs.waitForExistence(timeout: 10))
        theirs.tap()
        XCTAssertTrue(textLabel.waitForExistence(timeout: 15), "問題のフォームが開かない")
        XCTAssertFalse(app.staticTexts["正解"].exists, "作者以外のフォームに正解欄が出ている")
        XCTAssertTrue(app.staticTexts["配点"].exists || app.otherElements["配点"].exists || app.steppers.count > 0, "他の項目は出る")
        shot("3-form-other")
    }
}
