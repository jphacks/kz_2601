import XCTest
import MetamonDSL
import MetamonServices
@testable import Metamon

/// #313 β の初回の案内: β の版の判定(TestFlight は差し込む)・初回 1 回・順序(チュートリアルの後・ダウンロード確認の前)。
@MainActor
final class BetaWelcomeTests: XCTestCase {
    private var defaults: UserDefaults!
    private var suite = ""

    override func setUp() {
        suite = "betaWelcome313-\(UUID().uuidString)"
        defaults = UserDefaults(suiteName: suite)
    }

    override func tearDown() {
        defaults.removePersistentDomain(forName: suite)
        defaults = nil
    }

    // MARK: - β の版の判定

    func testTestFlightReleaseIsBeta() {
        XCTAssertTrue(BetaEnvironment.isBeta(isTestFlight: true, isReviewBuild: false, isDebugBuild: false, forceBeta: false))
    }

    func testAppStoreIsNeverBeta() {
        XCTAssertFalse(BetaEnvironment.isBeta(isTestFlight: false, isReviewBuild: false, isDebugBuild: false, forceBeta: false))
    }

    func testReviewBuildIsNeverBeta() {
        XCTAssertFalse(BetaEnvironment.isBeta(isTestFlight: true, isReviewBuild: true, isDebugBuild: false, forceBeta: false))
        XCTAssertFalse(BetaEnvironment.isBeta(isTestFlight: true, isReviewBuild: true, isDebugBuild: true, forceBeta: true))
    }

    func testDebugOnlyWhenForced() {
        XCTAssertFalse(BetaEnvironment.isBeta(isTestFlight: true, isReviewBuild: false, isDebugBuild: true, forceBeta: false),
                       "Xcode・シミュレータもレシートが sandbox なので、既存の UI テストを塞がない")
        XCTAssertTrue(BetaEnvironment.isBeta(isTestFlight: false, isReviewBuild: false, isDebugBuild: true, forceBeta: true))
    }

    func testReceiptDetection() {
        let base = URL(fileURLWithPath: "/var/mobile/Containers/Data/Application/X/StoreKit")
        XCTAssertTrue(BetaEnvironment.isSandboxReceipt(base.appendingPathComponent("sandboxReceipt")))
        XCTAssertFalse(BetaEnvironment.isSandboxReceipt(base.appendingPathComponent("receipt")))
        XCTAssertFalse(BetaEnvironment.isSandboxReceipt(nil))
    }

    // MARK: - 出すか

    func testDecision() {
        XCTAssertTrue(BetaWelcomeStore.shouldAutoPresent(isBeta: true, hasSeen: false, tutorialSeen: true), "β・未読 → 出す")
        XCTAssertFalse(BetaWelcomeStore.shouldAutoPresent(isBeta: true, hasSeen: true, tutorialSeen: true), "1 回だけ")
        XCTAssertFalse(BetaWelcomeStore.shouldAutoPresent(isBeta: true, hasSeen: false, tutorialSeen: false), "チュートリアルより先に出さない")
        XCTAssertFalse(BetaWelcomeStore.shouldAutoPresent(isBeta: false, hasSeen: false, tutorialSeen: true), "App Store・審査用は出さない")
    }

    func testStorePersistsSeenWithVersionedKey() {
        XCTAssertTrue(BetaWelcomeStore.seenKey.hasSuffix(".v1"))
        let store = BetaWelcomeStore(defaults: defaults, isBeta: true)
        XCTAssertTrue(store.shouldAutoPresent(tutorialSeen: true))
        store.markSeen()
        XCTAssertFalse(store.shouldAutoPresent(tutorialSeen: true))
        XCTAssertTrue(BetaWelcomeStore(defaults: defaults, isBeta: true).hasSeen)
    }

    func testLaunchOverride() {
        let store = BetaWelcomeStore(defaults: defaults, isBeta: true)
        store.applyLaunchOverride(environment: ["METAMON_BETA_WELCOME": "seen"])
        XCTAssertTrue(store.hasSeen)
        store.applyLaunchOverride(environment: ["METAMON_BETA_WELCOME": "reset"])
        XCTAssertFalse(store.hasSeen)
        store.applyLaunchOverride(environment: [:])
        XCTAssertFalse(store.hasSeen, "指定なしでは何もしない")
    }

    // MARK: - 順序(AppRuntime)

    private func makeRuntime(isBeta: Bool) throws -> AppRuntime {
        let rt = AppRuntime(document: try DSLExamples.document(DSLExamples.initial, version: 0),
                            services: BuildConfig.make(environment: .mock))
        rt.aiConsent = AIConsentStore(defaults: defaults)
        rt.tutorial = FirstRunTutorialStore(defaults: defaults)
        rt.betaWelcome = BetaWelcomeStore(defaults: defaults, isBeta: isBeta)
        return rt
    }

    func testRuntimeOrderAfterTutorialBeforeModelDownload() throws {
        let rt = try makeRuntime(isBeta: true)
        rt.showModelDownloadConfirm = true

        rt.presentBetaWelcomeIfNeeded()
        XCTAssertFalse(rt.showBetaWelcome, "チュートリアルを見る前は出さない")

        rt.finishTutorial()
        XCTAssertFalse(rt.canShowModelDownloadConfirm, "β の案内より先にダウンロード確認を出さない")
        rt.presentBetaWelcomeIfNeeded()
        XCTAssertTrue(rt.showBetaWelcome)
        XCTAssertTrue(rt.canOpenBetaWelcome)

        rt.finishBetaWelcome()
        XCTAssertFalse(rt.showBetaWelcome)
        XCTAssertTrue(rt.canShowModelDownloadConfirm, "案内を閉じたらダウンロード確認")
        rt.presentBetaWelcomeIfNeeded()
        XCTAssertFalse(rt.showBetaWelcome, "2 回目は自動で出さない")
    }

    func testRuntimeNotBetaNeverShows() throws {
        let rt = try makeRuntime(isBeta: false)
        rt.finishTutorial()
        rt.presentBetaWelcomeIfNeeded()
        XCTAssertFalse(rt.showBetaWelcome)
        XCTAssertFalse(rt.canOpenBetaWelcome, "設定にも出さない")
        XCTAssertFalse(rt.isBetaWelcomePending)
    }

    func testContentKeysAreLocalized() {
        guard let enPath = Bundle(for: AppRuntime.self).path(forResource: "en", ofType: "lproj"), let en = Bundle(path: enPath) else {
            return XCTFail("en.lproj が無い")
        }
        for key in BetaWelcomeContent.tryKeys + BetaWelcomeContent.limitKeys {
            let value = en.localizedString(forKey: key, value: "__missing__", table: nil)
            XCTAssertNotEqual(value, "__missing__", "英語が無い: \(key)")
            XCTAssertNotEqual(value, key, "英語が日本語のまま: \(key)")
        }
    }
}
