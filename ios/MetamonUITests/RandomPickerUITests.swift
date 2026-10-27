import XCTest

/// `randomPicker`(#263)の UI テスト: モック(METAMON_MOCK=1・晩ごはんの候補 4 件の seed 入り)で docs/dsl/examples/dinner.json を開き、
/// 「今日の晩ごはんを決める」を押す → 候補のどれかが出る → もう一度押すと別の 1 件 → 結果のタップでその料理のフォームが開く。
/// `RANDOM_SHOT_DIR=<dir>`(xcodebuild では `TEST_RUNNER_RANDOM_SHOT_DIR`)でスクショを PNG でも書き出す。
final class RandomPickerUITests: XCTestCase {
    private var app: XCUIApplication!
    private let dishes = ["カレー", "肉じゃが", "麻婆豆腐", "焼き魚"]

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
        if let dir = ProcessInfo.processInfo.environment["RANDOM_SHOT_DIR"], !dir.isEmpty {
            try? s.pngRepresentation.write(to: URL(fileURLWithPath: dir).appending(path: "\(name).png"))
        }
    }

    /// 演出(約 0.7 秒)が終わって結果が候補のどれかになるまで待ち、その名前を返す。
    private func waitForPick(different previous: String? = nil) -> String {
        let result = app.descendants(matching: .any)["randomPicker.result"].firstMatch
        let done = NSPredicate { obj, _ in
            guard let v = (obj as? XCUIElement)?.value as? String else { return false }
            return self.dishes.contains(v) && v != previous
        }
        expectation(for: done, evaluatedWith: result)
        waitForExpectations(timeout: 10)
        return result.value as? String ?? ""
    }

    func testPickShowsCandidateChangesAndOpensForm() {
        app = XCUIApplication()
        app.launchEnvironment = ["METAMON_MOCK": "1", "METAMON_FAB_POSITION": "auto", "METAMON_INITIAL_EXAMPLE": "dinner"]
        app.launchArguments = ["-AppleLanguages", "(ja)", "-AppleLocale", "ja_JP", "-metamon.appLanguage", "ja"]
        app.launch()

        let button = app.buttons["randomPicker.button"]
        XCTAssertTrue(button.waitForExistence(timeout: 20), "決めるボタンが無い")
        XCTAssertTrue(button.label.contains("今日の晩ごはんを決める"), "label が出ていない: \(button.label)")
        XCTAssertTrue(app.staticTexts["ボタンを押すと 1 つ選びます"].exists, "選ぶ前の案内が無い")
        shot("1-home")

        button.tap()
        let first = waitForPick()
        shot("2-picked")

        button.tap()
        let second = waitForPick(different: first)
        XCTAssertNotEqual(first, second, "続けて同じ料理が出た")
        shot("3-picked-again")

        app.descendants(matching: .any)["randomPicker.result"].firstMatch.tap()
        let name = app.textFields.matching(NSPredicate(format: "value == %@", second)).firstMatch
        XCTAssertTrue(name.waitForExistence(timeout: 15), "選ばれた料理(\(second))のフォームが開かない")
        shot("4-form")
    }
}
