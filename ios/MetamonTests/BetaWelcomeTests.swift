import XCTest
import MetamonDSL
import MetamonServices
@testable import Metamon

/// #313 β の初回の案内: β の版の判定(TestFlight は差し込む)・初回 1 回・順序(最初のアプリの後・H1)。
@MainActor
final class BetaWelcomeTests: XCTestCase {
    private var defaults: UserDefaults!
    private var suite = ""

    override func setUp() {
        suite = "betaWelcome313-\(UUID().uuidString)"
        defaults = UserDefaults(suiteName: suite)
        // 起動の順番待ちは共有の状態。テストホストの起動(同意の段)や他のテストの残りで止まらないよう空にする。
        SplashPresence.shared.resetForTesting()
        LaunchConsentHold.shared.resetForTesting()
    }

    override func tearDown() {
        SplashPresence.shared.resetForTesting()
        LaunchConsentHold.shared.resetForTesting()
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
        XCTAssertTrue(BetaWelcomeStore.shouldAutoPresent(isBeta: true, hasSeen: false, tutorialSeen: true, firstAppMade: true), "β・未読・最初のアプリの後 → 出す")
        XCTAssertFalse(BetaWelcomeStore.shouldAutoPresent(isBeta: true, hasSeen: false, tutorialSeen: true, firstAppMade: false),
                       "最初のアプリの前は出さない(H1: 初回の関門から外す)")
        XCTAssertFalse(BetaWelcomeStore.shouldAutoPresent(isBeta: true, hasSeen: true, tutorialSeen: true, firstAppMade: true), "1 回だけ")
        XCTAssertFalse(BetaWelcomeStore.shouldAutoPresent(isBeta: true, hasSeen: false, tutorialSeen: false, firstAppMade: true), "チュートリアルより先に出さない")
        XCTAssertFalse(BetaWelcomeStore.shouldAutoPresent(isBeta: false, hasSeen: false, tutorialSeen: true, firstAppMade: true), "App Store・審査用は出さない")
    }

    func testStorePersistsSeenWithVersionedKey() {
        XCTAssertTrue(BetaWelcomeStore.seenKey.hasSuffix(".v1"))
        let store = BetaWelcomeStore(defaults: defaults, isBeta: true)
        XCTAssertTrue(store.shouldAutoPresent(tutorialSeen: true, firstAppMade: true))
        store.markSeen()
        XCTAssertFalse(store.shouldAutoPresent(tutorialSeen: true, firstAppMade: true))
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

    /// H1・U6: β の案内は初回の関門から外し、最初のアプリの後に出す。音声モデルの確認は待たせない
    func testRuntimeShowsAfterFirstAppNotDuringOnboarding() throws {
        let rt = try makeRuntime(isBeta: true)
        rt.presentBetaWelcomeIfNeeded()
        XCTAssertFalse(rt.showBetaWelcome, "チュートリアルを見る前は出さない")

        rt.finishTutorial()
        rt.presentBetaWelcomeIfNeeded()
        XCTAssertFalse(rt.showBetaWelcome, "チュートリアルの直後(まだ何も作っていない)は出さない")
        rt.showModelDownloadConfirm = true
        XCTAssertTrue(rt.canShowModelDownloadConfirm, "β の案内を待たずにダウンロード確認を出せる")
        rt.showModelDownloadConfirm = false

        rt.tutorial?.markEditHintShown() // 最初のアプリができた
        rt.showFirstEditHint = true
        rt.presentBetaWelcomeIfNeeded()
        XCTAssertFalse(rt.showBetaWelcome, "直し方の例が出ている間は出さない")
        rt.showFirstEditHint = false
        rt.presentBetaWelcomeIfNeeded()
        XCTAssertTrue(rt.showBetaWelcome, "直し方の例を閉じたら出す")
        XCTAssertTrue(rt.canOpenBetaWelcome)

        rt.finishBetaWelcome()
        XCTAssertFalse(rt.showBetaWelcome)
        rt.presentBetaWelcomeIfNeeded()
        XCTAssertFalse(rt.showBetaWelcome, "2 回目は自動で出さない")
    }

    func testRuntimeWaitsForVoiceSheet() throws {
        let rt = try makeRuntime(isBeta: true)
        rt.finishTutorial()
        rt.tutorial?.markEditHintShown()
        rt.showVoiceSheet = true // 直し方の例の「話してみる」
        rt.presentBetaWelcomeIfNeeded()
        XCTAssertFalse(rt.showBetaWelcome, "音声シートの上には出さない(次の起動で出す)")
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
