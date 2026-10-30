import XCTest

/// 親から子を作る(#256)の UI テスト: モック(METAMON_MOCK=1・投稿 2 件の seed 入り)で docs/dsl/examples/post_comments.json を開き、
/// 投稿の行 → 投稿のフォーム →「コメントを書く」→ その投稿が入ったコメントの新規フォーム → 保存。新規の投稿からは先に保存を促す。
/// `PARENT_SHOT_DIR=<dir>`(xcodebuild では `TEST_RUNNER_PARENT_SHOT_DIR`)でスクショを PNG でも書き出す。
final class PrefillRefUITests: XCTestCase {
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
        if let dir = ProcessInfo.processInfo.environment["PARENT_SHOT_DIR"], !dir.isEmpty {
            try? s.pngRepresentation.write(to: URL(fileURLWithPath: dir).appending(path: "\(name).png"))
        }
    }

    private func launch() {
        app = XCUIApplication()
        app.launchEnvironment = ["METAMON_MOCK": "1", "METAMON_FAB_POSITION": "auto", "METAMON_AI_CONSENT": "granted", "METAMON_INITIAL_EXAMPLE": "post_comments"]
        app.launchArguments = ["-AppleLanguages", "(ja)", "-AppleLocale", "ja_JP", "-metamon.appLanguage", "ja"]
        app.launch()
    }

    func testWriteCommentFromPostPrefillsParent() {
        launch()
        let post = app.staticTexts["金沢でハッカソンに参加中"].firstMatch
        XCTAssertTrue(post.waitForExistence(timeout: 20), "seed の投稿が一覧に無い")
        shot("1-home")
        post.tap()

        let write = app.buttons["コメントを書く"].firstMatch
        XCTAssertTrue(write.waitForExistence(timeout: 15), "投稿のフォームが開かない")
        shot("2-post-form")
        write.tap()

        let parent = app.buttons.matching(NSPredicate(format: "label == %@ AND value == %@", "投稿", "金沢でハッカソンに参加中")).firstMatch
        XCTAssertTrue(parent.waitForExistence(timeout: 15), "コメントのフォームに親の投稿が入っていない")
        let text = app.textFields.matching(NSPredicate(format: "placeholderValue == 'いいですね'")).firstMatch
        XCTAssertTrue(text.exists, "コメントの入力欄が無い")
        let typed = text.value as? String ?? ""
        XCTAssertTrue(typed.isEmpty || typed == "いいですね", "新規で開く(既存のコメントを編集しない): \(typed)")
        shot("3-comment-form-prefilled")

        text.tap()
        text.typeText("がんばって")
        app.buttons["保存"].firstMatch.tap()
        XCTAssertTrue(parent.waitForNonExistence(timeout: 10), "保存でシートが閉じない")

        app.buttons["コメント"].firstMatch.tap()
        XCTAssertTrue(app.staticTexts["がんばって"].firstMatch.waitForExistence(timeout: 15), "コメントが保存されていない")
        shot("4-comments")
    }

    func testUnsavedPostAsksToSaveFirst() {
        launch()
        XCTAssertTrue(app.staticTexts["金沢でハッカソンに参加中"].firstMatch.waitForExistence(timeout: 20))
        app.navigationBars.buttons["投稿"].firstMatch.tap()

        let write = app.buttons["コメントを書く"].firstMatch
        XCTAssertTrue(write.waitForExistence(timeout: 15), "新規の投稿フォームが開かない")
        write.tap()
        let toast = app.staticTexts["先に「投稿」を保存してください"].firstMatch
        XCTAssertTrue(toast.waitForExistence(timeout: 5), "未保存の親で toast が出ない")
        XCTAssertEqual(app.staticTexts.matching(identifier: "先に「投稿」を保存してください").count, 1, "toast が二重に出ている(ToastHost の重複)")
        XCTAssertTrue(toast.isHittable, "toast がシートの下に隠れている")
        shot("5-unsaved-parent-toast")
        XCTAssertFalse(app.textFields.matching(NSPredicate(format: "placeholderValue == 'いいですね'")).firstMatch.exists, "コメントのフォームを開かない")
    }
}
