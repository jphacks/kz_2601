import XCTest

/// `chatThread`(#260)の UI テスト: モック(METAMON_MOCK=1・会話の seed 入り)でチャット(docs/dsl/examples/chat.json)を開き、
/// 相手(左・名前付き)と自分(右)の吹き出しが古い順に並ぶ → 下の入力欄から送る → 自分の吹き出しが一番下に増える。
/// `CHAT_SHOT_DIR=<dir>`(xcodebuild では `TEST_RUNNER_CHAT_SHOT_DIR`)でスクショを PNG でも書き出す。
final class ChatThreadUITests: XCTestCase {
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
        if let dir = ProcessInfo.processInfo.environment["CHAT_SHOT_DIR"], !dir.isEmpty {
            try? s.pngRepresentation.write(to: URL(fileURLWithPath: dir).appending(path: "\(name).png"))
        }
    }

    func testBubblesAndSendFromPinnedInput() {
        app = XCUIApplication()
        app.launchEnvironment = ["METAMON_MOCK": "1", "METAMON_FAB_POSITION": "auto", "METAMON_AI_CONSENT": "granted", "METAMON_INITIAL_EXAMPLE": "chat"]
        app.launchArguments = ["-AppleLanguages", "(ja)", "-AppleLocale", "ja_JP", "-metamon.appLanguage", "ja"]
        app.launch()

        // seed: さくら 2 通 → 自分 → たけし → さくら(古い順に上から)
        let others = app.descendants(matching: .any).matching(identifier: "chatThread.message.other")
        let mine = app.descendants(matching: .any).matching(identifier: "chatThread.message.mine")
        XCTAssertTrue(others.firstMatch.waitForExistence(timeout: 20), "相手の吹き出しが無い")
        XCTAssertTrue(mine.firstMatch.exists, "自分の吹き出しが無い")
        XCTAssertTrue(others.firstMatch.label.hasPrefix("さくら"), "相手の吹き出しに名前が無い: \(others.firstMatch.label)")
        let first = others.firstMatch.frame, me = mine.firstMatch.frame
        XCTAssertLessThan(first.minY, me.minY, "古い順(上から下)になっていない")
        XCTAssertGreaterThan(me.midX, first.midX, "自分の吹き出しが右に無い")
        shot("1-thread")

        let input = app.textFields["chatThread.input"]
        XCTAssertTrue(input.exists, "下の入力欄が無い")
        let send = app.buttons["chatThread.send"]
        XCTAssertFalse(send.isEnabled, "空のまま送れてしまう")
        input.tap()
        input.typeText("了解です、10 時に行きます")
        XCTAssertTrue(send.isEnabled)
        shot("2-typing")
        send.tap()

        // 共有アプリへの初めての投稿なら表示名を聞かれる(#250)
        let anonymous = app.buttons["匿名で投稿"]
        if anonymous.waitForExistence(timeout: 3) { anonymous.tap() }

        let sent = mine.matching(NSPredicate(format: "label CONTAINS %@", "了解です、10 時に行きます")).firstMatch
        XCTAssertTrue(sent.waitForExistence(timeout: 10), "送ったメッセージが自分の吹き出しに出ない")
        XCTAssertFalse((input.value as? String ?? "").contains("了解です"), "送信後に入力欄が空にならない")
        let last = others.allElementsBoundByIndex.map(\.frame.maxY).max() ?? 0
        XCTAssertGreaterThan(sent.frame.minY, last, "新しいメッセージが一番下に無い")
        shot("3-sent")
    }
}
