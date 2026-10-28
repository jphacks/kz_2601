import XCTest

/// `routeGuide`(#411)の UI テスト: モック(METAMON_MOCK=1・行き先 2 件の seed・現在地は金沢駅・経路は L 字の決まった線)で
/// docs/dsl/examples/drive.json を開き、行き先を選ぶ → 案内を開始 → 次に曲がる所・残りの距離・読み上げの文が出る → 到着、
/// 徒歩への切り替え・音声オフも確かめる。実機の GPS・MKDirections・読み上げの音は確かめない(モックの経路を流すだけ)。
/// `ROUTE_SHOT_DIR=<dir>`(xcodebuild では `TEST_RUNNER_ROUTE_SHOT_DIR`)でスクショを PNG でも書き出す。
final class RouteGuideUITests: XCTestCase {
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
        if let dir = ProcessInfo.processInfo.environment["ROUTE_SHOT_DIR"], !dir.isEmpty {
            try? s.pngRepresentation.write(to: URL(fileURLWithPath: dir).appending(path: "\(name).png"))
        }
    }

    private func element(_ id: String) -> XCUIElement { app.descendants(matching: .any)[id].firstMatch }

    func testGuideToChosenSpotUntilArrival() {
        app = XCUIApplication()
        app.launchEnvironment = ["METAMON_MOCK": "1", "METAMON_FAB_POSITION": "auto", "METAMON_INITIAL_EXAMPLE": "drive"]
        app.launchArguments = ["-AppleLanguages", "(ja)", "-AppleLocale", "ja_JP", "-metamon.appLanguage", "ja"]
        app.launch()

        // 音声の設定は引数で渡さない(引数のドメインが勝ち、オフにしても読めなくなる)。前回オフのまま終わっていたらオンに戻す
        let start = app.buttons["routeGuide.start"].firstMatch
        XCTAssertTrue(start.waitForExistence(timeout: 20), "案内を開始のボタンが無い")
        // 行き先は seed の先頭(兼六園)。メニューで近江町市場に変える
        let destination = element("routeGuide.destination")
        XCTAssertTrue(destination.waitForExistence(timeout: 10))
        destination.tap()
        let other = app.buttons["近江町市場"].firstMatch
        XCTAssertTrue(other.waitForExistence(timeout: 5), "行き先の候補が出ない")
        other.tap()
        let voice = app.buttons["routeGuide.voice"].firstMatch
        if voice.label.contains("オンにする") { voice.tap() }
        shot("1-home")

        start.tap()
        let banner = element("routeGuide.banner")
        XCTAssertTrue(banner.waitForExistence(timeout: 15), "案内の表示(次に曲がる所)が出ない")
        XCTAssertTrue(banner.label.contains("残り"), "残りの距離が無い: \(banner.label)")
        let cue = element("routeGuide.cue")
        XCTAssertTrue(cue.waitForExistence(timeout: 5), "読み上げた文が出ない")
        XCTAssertTrue(cue.label.contains("先、"), "最初の読み上げが「n メートル先、〜」でない: \(cue.label)")
        shot("2-guiding")

        // 案内中に徒歩へ切り替えても続く
        app.buttons["徒歩"].firstMatch.tap()
        XCTAssertTrue(banner.waitForExistence(timeout: 10))
        // 音声オフで読み上げた文の表示も消える
        app.buttons["routeGuide.voice"].firstMatch.tap()
        XCTAssertFalse(cue.waitForExistence(timeout: 2), "音声オフでも読み上げの文が出ている")

        XCTAssertTrue(element("routeGuide.arrived").waitForExistence(timeout: 60), "到着にならない")
        shot("3-arrived")
        app.buttons["routeGuide.stop"].firstMatch.tap()
        XCTAssertTrue(start.waitForExistence(timeout: 5), "終了で最初の状態に戻らない")
    }
}
