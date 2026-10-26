import XCTest

/// 新着(#268)の UI テスト: モックで他の人のレコード 2 件 + 自分の 1 件を返し(`METAMON_MOCK_NEW_ITEMS=2`)、
/// 開いた直後に「新着 2 件」のバナーが出て、タップで消えることを確かめる。
/// `NEWITEMS_SHOT_DIR=<dir>`(xcodebuild では `TEST_RUNNER_NEWITEMS_SHOT_DIR`)でスクショを PNG でも書き出す。
final class NewItemsUITests: XCTestCase {
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
        if let dir = ProcessInfo.processInfo.environment["NEWITEMS_SHOT_DIR"], !dir.isEmpty {
            try? s.pngRepresentation.write(to: URL(fileURLWithPath: dir).appending(path: "\(name).png"))
        }
    }

    func testBannerShowsOthersCountOnOpen() {
        app = XCUIApplication()
        app.launchEnvironment = ["METAMON_MOCK": "1", "METAMON_AI_CONSENT": "granted", "METAMON_INITIAL_EXAMPLE": "timetable",
                                 "METAMON_MOCK_NEW_ITEMS": "2"]
        app.launchArguments = ["-AppleLanguages", "(ja)", "-AppleLocale", "ja_JP", "-metamon.appLanguage", "ja"]
        app.launch()

        let banner = app.descendants(matching: .any)["newItems.banner"]
        let appeared = banner.waitForExistence(timeout: 20)
        if !appeared { shot("newitems-missing") }
        XCTAssertTrue(appeared, "新着のバナーが出ない")
        XCTAssertTrue(banner.label.contains("新着 2 件"), "自分のレコードを数えている / 件数が違う: \(banner.label)")
        shot("newitems-banner")

        banner.tap()
        let gone = NSPredicate(format: "exists == false")
        expectation(for: gone, evaluatedWith: banner)
        waitForExpectations(timeout: 5)
    }
}
