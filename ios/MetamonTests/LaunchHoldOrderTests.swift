import XCTest
import MetamonDSL
import MetamonServices
@testable import Metamon

/// 起動時の順番: スプラッシュ → 同意の画面 → チュートリアル → シェアリンク → Intent。β の案内・更新のお知らせもスプラッシュの後。
/// シートを同時に出すと SwiftUI が片方を捨て、同意が出ないまま止まるため。
@MainActor
final class LaunchHoldOrderTests: XCTestCase {
    private var defaults: UserDefaults!
    private var suite = ""

    override func setUp() {
        super.setUp()
        suite = "launchHold-\(UUID().uuidString)"
        defaults = UserDefaults(suiteName: suite)
        SplashPresence.shared.resetForTesting()
        LaunchConsentHold.shared.resetForTesting()
    }

    override func tearDown() {
        SplashPresence.shared.resetForTesting()
        LaunchConsentHold.shared.resetForTesting()
        #if !APP_STORE_REVIEW
        SharingUI.shared.isReady = true
        _ = SharingUI.shared.takeLink()
        SharingUI.shared.isReady = false
        SharingUI.shared.pendingImport = nil
        SharingUI.shared.pendingJoin = nil
        #endif
        defaults.removePersistentDomain(forName: suite)
        defaults = nil
        super.tearDown()
    }

    private func makeRuntime(isBeta: Bool = false, tutorialSeen: Bool = true) throws -> AppRuntime {
        let rt = AppRuntime(document: try DSLExamples.document(DSLExamples.initial, version: 0),
                            services: BuildConfig.make(environment: .mock))
        rt.aiConsent = AIConsentStore(defaults: defaults)
        rt.tutorial = FirstRunTutorialStore(defaults: defaults)
        if tutorialSeen { rt.tutorial?.markTutorialSeen() }
        rt.betaWelcome = BetaWelcomeStore(defaults: defaults, isBeta: isBeta)
        return rt
    }

    private func settle() async {
        for _ in 0..<5 { await Task.yield() }
    }

    // MARK: - 同意 → Intent

    func testIntentWaitsForConsentThenRunsAfterDecline() async throws {
        let rt = try makeRuntime()
        let hold = LaunchConsentHold.shared
        rt.beginLaunchConsentHoldIfNeeded()
        XCTAssertTrue(hold.isHolding, "初回(同意の画面を出す)は待たせる")

        let host = IntentHost()
        host.register(rt)
        host.markReady()
        rt.showAppSwitcher = true
        var done = false
        let intent = Task { @MainActor in
            await host.request(.talk)
            done = true
        }
        await settle()
        XCTAssertFalse(done, "同意の画面より先に Intent を開かない")
        XCTAssertTrue(rt.showAppSwitcher)

        await rt.presentLaunchConsentIfNeeded(delay: .zero)
        XCTAssertTrue(rt.showAIConsent, "同意の画面が先")
        XCTAssertTrue(hold.isHolding, "出ている間は待たせたまま")
        await settle()
        XCTAssertFalse(done)

        // 「あとで」で閉じる(同意しない)
        rt.aiConsent?.markPrompted()
        rt.showAIConsent = false
        rt.launchConsentDismissed()
        await intent.value
        XCTAssertTrue(done, "断っても止まらない(待たせていた Intent を出す)")
        XCTAssertFalse(rt.showAppSwitcher, "話しかける(.talk)が実行された")
        XCTAssertFalse(hold.isHolding)
    }

    func testNoHoldWhenConsentAlreadyGranted() throws {
        let rt = try makeRuntime()
        rt.aiConsent?.markPrompted()
        rt.grantAIConsent()
        rt.beginLaunchConsentHoldIfNeeded()
        XCTAssertFalse(LaunchConsentHold.shared.isHolding, "同意済みなら待たせない")
    }

    func testHoldReleasedWhenConsentNoLongerNeeded() async throws {
        let rt = try makeRuntime()
        rt.beginLaunchConsentHoldIfNeeded()
        XCTAssertTrue(LaunchConsentHold.shared.isHolding)
        // 起動処理の間に別の入口で同意した等: 出さずに放す
        rt.aiConsent?.markPrompted()
        rt.grantAIConsent()
        await rt.presentLaunchConsentIfNeeded(delay: .zero)
        XCTAssertFalse(rt.showAIConsent)
        XCTAssertFalse(LaunchConsentHold.shared.isHolding, "出さない時はすぐ放す(止まらない)")
    }

    func testHoldReleasedWhenPresentationCancelled() async throws {
        let rt = try makeRuntime()
        rt.beginLaunchConsentHoldIfNeeded()
        let task = Task { @MainActor in await rt.presentLaunchConsentIfNeeded(delay: .seconds(30)) }
        await settle()
        task.cancel() // 出す前に画面が消えた(サインアウト等)
        await task.value
        XCTAssertFalse(rt.showAIConsent)
        XCTAssertFalse(LaunchConsentHold.shared.isHolding, "取り消されても待たせたままにしない")
    }

    func testConsentWaitsForSplash() async throws {
        let rt = try makeRuntime()
        rt.beginLaunchConsentHoldIfNeeded()
        SplashPresence.shared.begin()
        let task = Task { @MainActor in await rt.presentLaunchConsentIfNeeded(delay: .zero) }
        await settle()
        XCTAssertFalse(rt.showAIConsent, "スプラッシュの上には出さない")
        SplashPresence.shared.end()
        await task.value
        XCTAssertTrue(rt.showAIConsent)
    }

    // MARK: - 同意 → チュートリアル → シェアリンク → Intent

    private func startIntent(_ rt: AppRuntime, done: @escaping @MainActor () -> Void) -> Task<Void, Never> {
        let host = IntentHost()
        host.register(rt)
        host.markReady()
        return Task { @MainActor in
            await host.request(.talk)
            done()
        }
    }

    func testTutorialRunsAfterConsentAndBeforeIntent() async throws {
        let rt = try makeRuntime(tutorialSeen: false)
        let hold = LaunchConsentHold.shared
        rt.beginLaunchConsentHoldIfNeeded()
        XCTAssertEqual(hold.pending, [.consent, .tutorial], "初回は同意とチュートリアルの両方を待たせる")

        rt.showAppSwitcher = true
        var done = false
        let intent = startIntent(rt) { done = true }
        let tutorialStep = Task { @MainActor in await rt.runLaunchTutorialStep(settle: .zero) }
        await settle()
        XCTAssertFalse(rt.showTutorial, "同意の画面より先にチュートリアルを出さない")

        await rt.presentLaunchConsentIfNeeded(delay: .zero)
        XCTAssertTrue(rt.showAIConsent)
        rt.aiConsent?.markPrompted() // 「あとで」
        rt.showAIConsent = false
        rt.launchConsentDismissed()
        await tutorialStep.value
        XCTAssertTrue(rt.showTutorial, "同意が閉じきってからチュートリアル")
        XCTAssertTrue(hold.isHolding(before: .shareLink), "チュートリアルが出ている間はシェアリンクを開かない")
        await settle()
        XCTAssertFalse(done, "チュートリアルが出ている間は Intent を開かない")

        rt.finishTutorial()
        await settle()
        XCTAssertFalse(done, "閉じるアニメーションが終わる(onDismiss)まで待つ")
        rt.tutorialDismissed()
        await intent.value
        XCTAssertTrue(done)
        XCTAssertFalse(rt.showAppSwitcher, "話しかける(.talk)が実行された")
        XCTAssertFalse(hold.isHolding)
    }

    func testTutorialStepSkippedWhenAlreadySeen() async throws {
        let rt = try makeRuntime()
        rt.beginLaunchConsentHoldIfNeeded()
        XCTAssertEqual(LaunchConsentHold.shared.pending, [.consent], "見たチュートリアルは待たない")
        var done = false
        let intent = startIntent(rt) { done = true }
        await rt.presentLaunchConsentIfNeeded(delay: .zero)
        rt.aiConsent?.markPrompted()
        rt.showAIConsent = false
        rt.launchConsentDismissed()
        await rt.runLaunchTutorialStep(settle: .zero)
        XCTAssertFalse(rt.showTutorial)
        await intent.value
        XCTAssertTrue(done)
    }

    func testTutorialStepReleasedWhenNotPresented() async throws {
        // 同意の説明が済んでいない(同意の画面を出す前に取り消された等)ならチュートリアルは出ない → 止めずに次へ
        let rt = try makeRuntime(tutorialSeen: false)
        let hold = LaunchConsentHold.shared
        rt.beginLaunchConsentHoldIfNeeded()
        hold.end(.consent)
        await rt.runLaunchTutorialStep(settle: .zero)
        XCTAssertFalse(rt.showTutorial)
        XCTAssertFalse(hold.isHolding, "出さなかった時はすぐ放す")
    }

    func testTutorialStepReleasedWhenCancelled() async throws {
        let rt = try makeRuntime(tutorialSeen: false)
        let hold = LaunchConsentHold.shared
        rt.beginLaunchConsentHoldIfNeeded()
        let step = Task { @MainActor in await rt.runLaunchTutorialStep(settle: .seconds(30)) }
        await settle()
        step.cancel() // 画面が消えた
        rt.launchPresentationsAbandoned()
        await step.value
        XCTAssertFalse(rt.showTutorial)
        XCTAssertFalse(hold.isHolding, "取り消されても待たせたままにしない")
    }

    func testAbandonReleasesEveryStep() async throws {
        let rt = try makeRuntime(tutorialSeen: false)
        rt.beginLaunchConsentHoldIfNeeded()
        LaunchConsentHold.shared.begin(.shareLink)
        var done = false
        let intent = startIntent(rt) { done = true }
        await settle()
        XCTAssertFalse(done)
        rt.launchPresentationsAbandoned() // サインアウト等
        await intent.value
        XCTAssertTrue(done)
    }

    #if !APP_STORE_REVIEW
    func testShareLinkWaitsForTutorialAndIntentWaitsForLink() async throws {
        let rt = try makeRuntime(tutorialSeen: false)
        let hold = LaunchConsentHold.shared
        rt.aiConsent?.markPrompted() // 同意の説明は済み・チュートリアルだけ
        rt.beginLaunchConsentHoldIfNeeded()
        XCTAssertEqual(hold.pending, [.tutorial])

        let ui = SharingUI.shared
        XCTAssertTrue(ui.receive(URL(string: ShareURL.scheme + "://import/abc123")!))
        XCTAssertTrue(hold.isHolding(before: .shareLink), "チュートリアルが閉じるまでリンクを開かない")
        var done = false
        let intent = startIntent(rt) { done = true }

        await rt.runLaunchTutorialStep(settle: .zero)
        XCTAssertTrue(rt.showTutorial)
        rt.finishTutorial()
        rt.tutorialDismissed()
        XCTAssertFalse(hold.isHolding(before: .shareLink), "チュートリアルの後はリンクの番")
        await settle()
        XCTAssertFalse(done, "リンクの確認が終わるまで Intent を開かない")

        ui.isReady = true
        XCTAssertNotNil(ui.takeLink())
        ui.linkSettled() // 確認を出さなかった(モックはトースト)・または確認が閉じた
        await intent.value
        XCTAssertTrue(done)
        XCTAssertFalse(hold.isHolding)
    }

    func testLinkSettledWaitsWhileConfirmationIsOpen() throws {
        let hold = LaunchConsentHold.shared
        let ui = SharingUI.shared
        ui.receive(URL(string: ShareURL.scheme + "://import/abc123")!)
        ui.linkSettled()
        XCTAssertTrue(hold.isHolding, "まだ開いていないリンクがある間は放さない")
        ui.isReady = true
        _ = ui.takeLink()
        ui.pendingImport = PendingImport(token: "abc123", preview: nil)
        ui.linkSettled()
        XCTAssertTrue(hold.isHolding, "追加の確認が出ている間は放さない")
        ui.pendingImport = nil
        ui.linkSettled()
        XCTAssertFalse(hold.isHolding)
    }
    #endif

    // MARK: - β の案内・更新のお知らせ

    #if !APP_STORE_REVIEW
    func testBetaWelcomeHeldBehindSplashAndConsent() throws {
        let rt = try makeRuntime(isBeta: true, tutorialSeen: false)
        rt.finishTutorial()
        rt.tutorial?.markEditHintShown() // 最初のアプリの後
        XCTAssertTrue(rt.isBetaWelcomePending)

        SplashPresence.shared.begin()
        rt.presentBetaWelcomeIfNeeded()
        XCTAssertFalse(rt.showBetaWelcome, "スプラッシュの上には出さない")
        SplashPresence.shared.end()

        LaunchConsentHold.shared.begin()
        rt.presentBetaWelcomeIfNeeded()
        XCTAssertFalse(rt.showBetaWelcome, "起動時の同意の画面より先に出さない")
        LaunchConsentHold.shared.end()

        rt.presentBetaWelcomeIfNeeded()
        XCTAssertTrue(rt.showBetaWelcome)
    }
    #endif

    private struct StubClient: ClientConfigClient {
        let config: ClientConfig
        func fetch() async throws -> ClientConfig { config }
    }

    func testUpdateAlertHeldBehindSplash() async {
        let gate = UpdateGate(checker: UpdateChecker(client: StubClient(config: ClientConfig(latestIOSBuild: 20)), build: "10"),
                              dismissals: NoticeDismissals(defaults: defaults))
        await gate.check()
        XCTAssertTrue(gate.showsNotice)
        SplashPresence.shared.begin()
        XCTAssertFalse(gate.canShowNoticeAlert, "スプラッシュの上にアラートを出さない")
        SplashPresence.shared.end()
        XCTAssertTrue(gate.canShowNoticeAlert)
    }
}
