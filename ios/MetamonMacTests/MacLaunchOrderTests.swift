import XCTest
import MetamonServices
@testable import Metamon

/// Mac の起動時の画面の順番(#1885・iOS の LaunchHoldOrderTests と同じ決まり)。
/// 同意 → チュートリアル → シェアリンク → Intent を、前の画面が閉じきってから 1 つずつ。出さない段・断った段もすぐ次へ。
/// β の案内と更新のお知らせは全段の後。順番待ちは iOS と共有の `LaunchConsentHold`(テストごとに新しく作る)。
@MainActor
final class MacLaunchOrderTests: XCTestCase {
    private var defaults: UserDefaults!
    private var suite = ""
    private var hold: LaunchConsentHold!
    private var owner: MacLaunchOwner!

    override func setUp() {
        super.setUp()
        suite = "macLaunchOrder-\(UUID().uuidString)"
        defaults = UserDefaults(suiteName: suite)
        hold = LaunchConsentHold()
        owner = MacLaunchOwner()
    }

    override func tearDown() {
        hold.endAll()
        hold = nil
        defaults.removePersistentDomain(forName: suite)
        defaults = nil
        super.tearDown()
    }

    private func onboarding(isBeta: Bool = false) -> MacOnboarding {
        #if APP_STORE_REVIEW
        let made = MacOnboarding(consent: AIConsentStore(defaults: defaults), tutorial: FirstRunTutorialStore(defaults: defaults))
        #else
        let made = MacOnboarding(consent: AIConsentStore(defaults: defaults), tutorial: FirstRunTutorialStore(defaults: defaults),
                                 betaWelcome: BetaWelcomeStore(defaults: defaults, isBeta: isBeta))
        #endif
        made.hold = hold
        made.launchOwner = owner
        return made
    }

    private func settle() async {
        for _ in 0..<5 { await Task.yield() }
    }

    /// Intent(話しかける)を受け取る。全段が終わるまで待つ
    private func startIntent(done: @escaping @MainActor (IntentHost.Pending?) -> Void) -> Task<Void, Never> {
        let hold = self.hold!
        return Task { @MainActor in
            done(await MacIntentRoute.takeAfterLaunch({ .talk }, hold: hold))
        }
    }

    // MARK: - 同意 → チュートリアル → Intent

    func testFirstRunHoldsConsentAndTutorial() {
        let ob = onboarding()
        MacOnboarding.beginLaunchHold(consent: ob.consent, tutorial: ob.tutorial, hold: hold)
        XCTAssertEqual(hold.pending, [.consent, .tutorial])
    }

    func testNothingHeldWhenAlreadyDone() {
        let ob = onboarding()
        ob.consent.markPrompted()
        ob.consent.grant()
        ob.tutorial.markTutorialSeen()
        MacOnboarding.beginLaunchHold(consent: ob.consent, tutorial: ob.tutorial, hold: hold)
        XCTAssertFalse(hold.isHolding, "同意済み・見たチュートリアルは待たせない")
        XCTAssertTrue(hold.launchFinished, "後から届いたリンクは待たせない")
    }

    func testConsentThenTutorialThenIntentEvenWhenDeclined() async {
        let ob = onboarding()
        MacOnboarding.beginLaunchHold(consent: ob.consent, tutorial: ob.tutorial, hold: hold)
        var taken: IntentHost.Pending?
        var done = false
        let intent = startIntent { taken = $0; done = true }
        let tutorialStep = Task { @MainActor in await ob.runLaunchTutorialStep(settle: .zero) }
        await settle()
        XCTAssertFalse(ob.showTutorial, "同意の画面より先にチュートリアルを出さない")

        await ob.presentLaunchConsent(delay: .zero)
        XCTAssertTrue(ob.showConsent, "同意の画面が先")
        await settle()
        XCTAssertFalse(done, "同意の画面より先に Intent を開かない")

        // 「あとで」で閉じる(同意しない)
        ob.consent.markPrompted()
        ob.showConsent = false
        ob.launchConsentDismissed()
        await tutorialStep.value
        XCTAssertTrue(ob.showTutorial, "断っても止まらず、閉じきってからチュートリアル")
        XCTAssertTrue(hold.isHolding(before: .shareLink), "チュートリアルの間はシェアリンクを開かない")
        await settle()
        XCTAssertFalse(done, "チュートリアルの間は Intent を開かない")

        ob.finishTutorial()
        await settle()
        XCTAssertFalse(done, "閉じるアニメーションが終わる(onDismiss)まで待つ")
        ob.tutorialDismissed()
        await intent.value
        XCTAssertEqual(taken, .talk)
        XCTAssertFalse(hold.isHolding)
    }

    func testStepsNotShownMoveOnImmediately() async {
        let ob = onboarding()
        ob.tutorial.markTutorialSeen()
        hold.begin(.consent)
        hold.begin(.tutorial)
        // 起動の処理の間に別の入口で同意した等: 同意の画面は出さずに放す
        ob.consent.markPrompted()
        ob.consent.grant()
        await ob.presentLaunchConsent(delay: .zero)
        XCTAssertFalse(ob.showConsent)
        XCTAssertFalse(hold.pending.contains(.consent), "出さない段はすぐ次へ")
        await ob.runLaunchTutorialStep(settle: .zero)
        XCTAssertFalse(ob.showTutorial, "見たチュートリアルは出さない")
        XCTAssertFalse(hold.isHolding, "出さなかった段もすぐ放す")
    }

    func testConsentCancelledBeforeShownReleases() async {
        let ob = onboarding()
        hold.begin(.consent)
        let task = Task { @MainActor in await ob.presentLaunchConsent(delay: .seconds(30)) }
        await settle()
        task.cancel() // 出す前にウインドウが閉じた
        await task.value
        XCTAssertFalse(ob.showConsent)
        XCTAssertFalse(hold.isHolding, "取り消されても待たせたままにしない")
    }

    func testUnshownStepReleasedAfterDeadline() async {
        hold.deadlineOverride = .milliseconds(50)
        hold.begin(.consent)
        hold.begin(.tutorial)
        hold.hostsAppeared()
        await hold.waitUntilReleased()
        XCTAssertFalse(hold.isHolding, "シートが捨てられて出なくても、後ろがずっと止まらない")
    }

    /// ウインドウが 2 つ: 起動時の段は最初のウインドウだけが出す。ほかのウインドウは段を放さない
    func testOnlyFirstWindowRunsLaunchSteps() async {
        let first = onboarding()
        let second = onboarding()
        MacOnboarding.beginLaunchHold(consent: first.consent, tutorial: first.tutorial, hold: hold)
        await first.presentLaunchConsent(delay: .zero)
        XCTAssertTrue(first.showConsent)

        await second.presentLaunchConsent(delay: .zero)
        await second.runLaunchTutorialStep(settle: .zero)
        XCTAssertFalse(second.showConsent, "2 つ目のウインドウは同意の画面を出さない")
        XCTAssertFalse(second.showTutorial)
        XCTAssertEqual(hold.pending, [.consent, .tutorial], "最初のウインドウのシートが出ている間に段を放さない")
        XCTAssertFalse(second.ownsLaunchSteps)
        XCTAssertTrue(first.ownsLaunchSteps)
    }

    // MARK: - シェアリンク

    #if !APP_STORE_REVIEW
    func testShareLinkWaitsForTutorialAndIntentWaitsForLink() async {
        let flow = MacShareFlow(sharing: nil, appConfig: nil, isSignedIn: { true })
        flow.hold = hold
        hold.begin(.tutorial)
        flow.receive(URL(string: "https://tailor-it.app/s/tok_1")!)
        XCTAssertTrue(hold.pending.contains(.shareLink), "起動の途中のリンクは Intent を待たせる")
        XCTAssertTrue(flow.waitsForEarlierSteps, "チュートリアルが閉じるまで案内を出さない")
        var done = false
        let intent = startIntent { _ in done = true }

        hold.end(.tutorial)
        XCTAssertFalse(flow.waitsForEarlierSteps, "チュートリアルの後はリンクの番")
        await settle()
        XCTAssertFalse(done, "リンクの案内が閉じるまで Intent を開かない")
        flow.linkSettled()
        XCTAssertTrue(hold.isHolding, "案内が出ている間は放さない")
        flow.unavailable = nil // 案内を閉じた
        flow.linkSettled()
        await intent.value
        XCTAssertTrue(done)
        XCTAssertFalse(hold.isHolding)
    }

    /// 「サーバー接続時のみ」の案内が出ている間は、見張りの時間が過ぎても Intent を出さない
    func testUnavailableAlertNotReleasedByDeadline() async throws {
        let flow = MacShareFlow(sharing: nil, appConfig: nil, isSignedIn: { true })
        flow.hold = hold
        hold.deadlineOverride = .milliseconds(30)
        flow.receive(URL(string: "https://tailor-it.app/s/tok_3")!)
        XCTAssertTrue(flow.showsUnavailable)
        flow.unavailableShown() // アラートが出た
        hold.hostsAppeared()
        try await Task.sleep(for: .milliseconds(200))
        XCTAssertTrue(hold.pending.contains(.shareLink), "案内が出ている間は時間で放さない")
        flow.unavailable = nil // 閉じた
        flow.linkSettled()
        XCTAssertFalse(hold.isHolding)
    }

    func testUnavailableNotMarkedShownWhileEarlierStepsRemain() async throws {
        let flow = MacShareFlow(sharing: nil, appConfig: nil, isSignedIn: { true })
        flow.hold = hold
        hold.deadlineOverride = .milliseconds(30)
        hold.begin(.tutorial)
        flow.receive(URL(string: "https://tailor-it.app/s/tok_4")!)
        XCTAssertFalse(flow.showsUnavailable, "チュートリアルの間は案内を出さない")
        flow.unavailableShown() // まだ出ていないので何もしない
        hold.hostsAppeared()
        await hold.waitUntilReleased()
        XCTAssertFalse(hold.isHolding, "出ていない段は見張りで放す")
    }

    func testShareLinkAfterLaunchDoesNotHold() {
        hold.finishLaunchIfIdle()
        let flow = MacShareFlow(sharing: nil, appConfig: nil, isSignedIn: { true })
        flow.hold = hold
        flow.receive(URL(string: "https://tailor-it.app/s/tok_2")!)
        XCTAssertFalse(hold.isHolding, "使っている途中に届いたリンクは待たせない")
    }

    // MARK: - β の案内・更新のお知らせ

    func testBetaWelcomeWaitsForLaunchSteps() {
        let ob = onboarding(isBeta: true)
        ob.consent.markPrompted()
        ob.tutorial.markTutorialSeen()
        ob.tutorial.markEditHintShown() // 最初のアプリの後
        hold.begin(.shareLink)
        ob.presentBetaWelcomeIfNeeded()
        XCTAssertFalse(ob.showBetaWelcome, "起動時の段が残っている間は出さない")
        hold.end(.shareLink)
        ob.presentBetaWelcomeIfNeeded()
        XCTAssertTrue(ob.showBetaWelcome)
    }
    #endif

    private struct ConfigClient: ClientConfigClient {
        var config: ClientConfig
        func fetch() async throws -> ClientConfig { config }
    }

    func testUpdateNoticeWaitsForLaunchSteps() async {
        let gate = MacUpdateGate(configuredStoreURL: nil)
        gate.hold = hold
        gate.apply(await UpdateChecker(client: ConfigClient(config: ClientConfig(latestMacOSBuild: 50)), build: "20", platform: .macos).check())
        XCTAssertTrue(gate.showsNotice)
        hold.begin(.consent)
        XCTAssertFalse(gate.canShowNoticeAlert, "同意のシートの前にアラートを出さない(シートが捨てられて止まる)")
        hold.end(.consent)
        XCTAssertTrue(gate.canShowNoticeAlert)
    }
}
