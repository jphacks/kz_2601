import XCTest
@testable import Metamon

/// 起動のスプラッシュを出す条件: コールド起動の 1 回だけ・視差効果を減らす・起動引数/モックで飛ばす。
final class SplashGateTests: XCTestCase {
    override func setUp() {
        super.setUp()
        SplashGate.resetForTesting()
    }

    override func tearDown() {
        SplashGate.resetForTesting()
        super.tearDown()
    }

    func testColdLaunchShowsOnceThenNeverAgainInSameProcess() {
        XCTAssertEqual(SplashGate.decide(arguments: [], environment: [:], reduceMotion: false), .animated)
        // 背景から戻る・シーンの作り直しでは出さない
        XCTAssertEqual(SplashGate.decide(arguments: [], environment: [:], reduceMotion: false), .hidden)
        XCTAssertTrue(SplashGate.didShowThisProcess)
    }

    func testReduceMotionUsesStaticPath() {
        XCTAssertEqual(SplashGate.decide(arguments: [], environment: [:], reduceMotion: true), .reduced)
    }

    func testLaunchArgumentSkipsAndDoesNotConsumeColdLaunch() {
        XCTAssertEqual(SplashGate.decide(arguments: ["-metamonSkipSplash"], environment: [:], reduceMotion: false), .hidden)
        XCTAssertFalse(SplashGate.didShowThisProcess)
    }

    func testMockAndTestHostSkipUnlessForced() {
        XCTAssertEqual(SplashGate.mode(arguments: [], environment: ["METAMON_MOCK": "1"], reduceMotion: false), .hidden)
        XCTAssertEqual(SplashGate.mode(arguments: [], environment: ["XCTestConfigurationFilePath": "/x"], reduceMotion: false), .hidden)
        XCTAssertEqual(SplashGate.mode(arguments: [], environment: ["METAMON_MOCK": "1", "METAMON_SPLASH": "1"], reduceMotion: false), .animated)
        XCTAssertEqual(SplashGate.mode(arguments: [], environment: ["METAMON_SPLASH": "1"], reduceMotion: true), .reduced)
        XCTAssertEqual(SplashGate.mode(arguments: [], environment: ["METAMON_SPLASH": "0"], reduceMotion: false), .hidden)
        // 起動引数は強制より強い
        XCTAssertEqual(SplashGate.mode(arguments: ["-metamonSkipSplash"], environment: ["METAMON_SPLASH": "1"], reduceMotion: false), .hidden)
    }

    func testFitsInBudget() {
        XCTAssertLessThanOrEqual(SplashView.Timing.finish + 0.3, 2.5)
        XCTAssertLessThan(SplashView.Timing.stitch, SplashView.Timing.word)
        XCTAssertLessThan(SplashView.Timing.icon, SplashView.Timing.word)
        XCTAssertLessThan(SplashView.Timing.word, SplashView.Timing.tag)
    }
}
