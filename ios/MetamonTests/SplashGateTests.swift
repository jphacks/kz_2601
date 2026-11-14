import XCTest
import MetamonDSL
import MetamonServices
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
        // 開始を待ちきれなかった時も、待つ + 完成形 + フェードで 3 秒に収める
        XCTAssertLessThanOrEqual(SplashStartGate.cap + SplashView.Timing.stillFinish + 0.3, 3.0)
    }

    // MARK: - VoiceOver

    func testVoiceOverSkipsSoFirstDoubleTapReachesScreen() {
        XCTAssertEqual(SplashGate.mode(arguments: [], environment: [:], reduceMotion: false, voiceOver: true), .hidden)
        XCTAssertEqual(SplashGate.mode(arguments: [], environment: ["METAMON_SPLASH": "1"], reduceMotion: false, voiceOver: true), .hidden)
        XCTAssertEqual(SplashGate.decide(arguments: [], environment: [:], reduceMotion: false, voiceOver: true), .hidden)
        XCTAssertFalse(SplashGate.didShowThisProcess)
    }

    // MARK: - 縫い始める合図(最初のコマ + メインスレッドが空いた)

    func testStartsAfterConsecutiveSmoothFrames() {
        var gate = SplashStartGate(origin: 10)
        XCTAssertEqual(gate.frame(at: 10.016), .wait, "最初のコマだけでは始めない")
        XCTAssertEqual(gate.frame(at: 10.033), .wait)
        XCTAssertEqual(gate.frame(at: 10.050), .wait)
        XCTAssertEqual(gate.frame(at: 10.066), .start, "続けて間に合ったら始める")
    }

    func testStallResetsTheCount() {
        var gate = SplashStartGate(origin: 0)
        _ = gate.frame(at: 0.016)
        _ = gate.frame(at: 0.033)
        _ = gate.frame(at: 0.050)
        XCTAssertEqual(gate.frame(at: 0.900), .wait, "メインスレッドが詰まった(間が空いた)ら数え直す")
        XCTAssertEqual(gate.frame(at: 0.916), .wait)
        XCTAssertEqual(gate.frame(at: 0.933), .wait)
        XCTAssertEqual(gate.frame(at: 0.950), .start)
    }

    func testFallsBackAtCap() {
        var gate = SplashStartGate(origin: 0)
        XCTAssertEqual(gate.frame(at: 0.2), .wait)
        XCTAssertEqual(gate.frame(at: SplashStartGate.cap + 0.1), .fallback, "1.5 秒待っても空かなければ完成形でフェード")
    }

    // MARK: - 開始を待った分だけアニメを縮める(全体 3 秒以内)

    func testPaceKeepsFullSpeedWhenStartIsQuick() {
        XCTAssertEqual(SplashStartGate.pace(waited: 0, animation: SplashView.Timing.finish), 1)
        XCTAssertEqual(SplashStartGate.pace(waited: 0.1, animation: SplashView.Timing.finish), 1, "すぐ始まればそのまま")
    }

    func testLateStartShortensAnimationToFitBudget() {
        let animation = SplashView.Timing.finish
        for waited in stride(from: 0.0, through: SplashStartGate.cap, by: 0.05) {
            let k = SplashStartGate.pace(waited: waited, animation: animation)
            XCTAssertLessThanOrEqual(k, 1)
            XCTAssertGreaterThan(k, 0)
            XCTAssertLessThanOrEqual(waited + animation * k + SplashStartGate.fadeOut, SplashStartGate.totalBudget + 1e-9,
                                     "\(waited) 秒待っても 3 秒に収める")
        }
        XCTAssertLessThan(SplashStartGate.pace(waited: 1.4, animation: animation), 1, "遅く始まったら縮める")
    }

    func testStartPathEndToEndStaysInBudget() {
        // 詰まって 1.2 秒目にやっと始まった: 合図 → 縮めたアニメ → フェード
        var gate = SplashStartGate(origin: 0)
        var started: TimeInterval?
        for time in [0.016, 0.9, 1.2, 1.216, 1.233, 1.25] where started == nil {
            if gate.frame(at: time) == .start { started = time }
        }
        let t = try? XCTUnwrap(started)
        XCTAssertEqual(t, 1.25)
        let k = SplashStartGate.pace(waited: t ?? 0, animation: SplashView.Timing.finish)
        XCTAssertLessThan(k, 1)
        XCTAssertLessThanOrEqual((t ?? 0) + SplashView.Timing.finish * k + SplashStartGate.fadeOut, 3.0 + 1e-9)
        // 待ちきれなかった時は完成形(縮めない)
        XCTAssertLessThanOrEqual(SplashStartGate.cap + SplashView.Timing.stillFinish + SplashStartGate.fadeOut, SplashStartGate.totalBudget)
    }

    // MARK: - スプラッシュの間は起動時のシート・リンク・吹き出しを待たせる

    @MainActor
    func testPresenceHoldsWaitersUntilEnd() async {
        let presence = SplashPresence()
        await presence.waitUntilHidden() // 出ていなければすぐ戻る
        presence.begin()
        var released = false
        let waiter = Task { @MainActor in
            await presence.waitUntilHidden()
            released = true
        }
        await Task.yield()
        await Task.yield()
        XCTAssertFalse(released, "出ている間は待つ")
        presence.end()
        await waiter.value
        XCTAssertTrue(released)
        XCTAssertFalse(presence.isShowing)
    }

    @MainActor
    func testCoachMarksWaitForSplash() throws {
        let suite = "splashCoach-\(UUID().uuidString)"
        let defaults = try XCTUnwrap(UserDefaults(suiteName: suite))
        defer { defaults.removePersistentDomain(forName: suite); SplashPresence.shared.resetForTesting() }
        let rt = AppRuntime(document: try DSLExamples.document(DSLExamples.initial, version: 0),
                            services: BuildConfig.make(environment: .mock))
        rt.aiConsent = AIConsentStore(defaults: defaults)
        rt.tutorial = FirstRunTutorialStore(defaults: defaults)
        rt.betaWelcome = BetaWelcomeStore(defaults: defaults, isBeta: false)
        rt.coachMarks = CoachMarkStore(defaults: defaults)
        rt.tutorial?.markTutorialSeen()
        rt.tutorial?.markEditHintShown()
        XCTAssertEqual(rt.fabCoachMark, .fabLongPress)
        SplashPresence.shared.begin()
        XCTAssertFalse(rt.canShowCoachMarks, "スプラッシュの下で出した記録・読み上げをしない")
        XCTAssertNil(rt.fabCoachMark)
        SplashPresence.shared.end()
        XCTAssertEqual(rt.fabCoachMark, .fabLongPress)
    }
}
