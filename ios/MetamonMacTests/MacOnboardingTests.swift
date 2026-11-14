import XCTest
import MetamonServices
@testable import Metamon

/// Mac の初回の案内の順(`MacOnboarding`・#971 / #2325): 同意 → チュートリアル → β の案内。
/// 起動時の順番待ちそのものは `MacLaunchOrderTests`。ここはどれを出すかの判定だけ。保存先はテストごとの UserDefaults。
@MainActor
final class MacOnboardingTests: XCTestCase {
    private var defaults: UserDefaults!
    private var suite = ""
    private var hold: LaunchConsentHold!

    override func setUp() {
        super.setUp()
        suite = "macOnboarding-\(UUID().uuidString)"
        defaults = UserDefaults(suiteName: suite)
        hold = LaunchConsentHold()
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
        made.launchOwner = MacLaunchOwner()
        return made
    }

    private var nothingShown: (MacOnboarding) -> Bool {
        { !$0.showConsent && !$0.showTutorial && !$0.showBetaWelcome }
    }

    // MARK: - 同意 → チュートリアル

    func testFirstRunStartsWithConsent() {
        let ob = onboarding()
        ob.presentTutorialIfNeeded()
        XCTAssertFalse(ob.showTutorial, "同意の説明より先にチュートリアルを出さない")
        ob.presentConsentIfNeeded()
        XCTAssertTrue(ob.showConsent, "初回は同意から")
        ob.presentTutorialIfNeeded()
        XCTAssertFalse(ob.showTutorial, "同意の画面が出ている間は重ねない")
    }

    func testTutorialAfterConsent() {
        let ob = onboarding()
        ob.consent.markPrompted()
        ob.consent.grant()
        ob.presentConsentIfNeeded()
        XCTAssertFalse(ob.showConsent, "同意済みならもう出さない")
        ob.presentTutorialIfNeeded()
        XCTAssertTrue(ob.showTutorial, "同意済みならチュートリアル")
    }

    func testTutorialAfterConsentPostponed() {
        let ob = onboarding()
        ob.consent.markPrompted() // 「あとで」
        ob.presentConsentIfNeeded()
        XCTAssertFalse(ob.showConsent, "「あとで」の人に起動のたびには出さない")
        ob.presentTutorialIfNeeded()
        XCTAssertTrue(ob.showTutorial, "説明を出し終えていればチュートリアルへ進む")
    }

    func testFinishTutorialIsRemembered() {
        let ob = onboarding()
        ob.consent.markPrompted()
        ob.presentTutorialIfNeeded()
        ob.finishTutorial()
        XCTAssertFalse(ob.showTutorial)
        XCTAssertTrue(ob.tutorial.hasSeenTutorial)
        let next = onboarding() // 次の起動
        next.presentTutorialIfNeeded()
        XCTAssertFalse(next.showTutorial, "見たチュートリアルは出さない")
    }

    func testAllDoneShowsNothing() {
        let ob = onboarding(isBeta: true)
        ob.consent.markPrompted()
        ob.consent.grant()
        ob.tutorial.markTutorialSeen()
        ob.tutorial.markEditHintShown()
        #if !APP_STORE_REVIEW
        ob.betaWelcome.markSeen()
        #endif
        ob.presentConsentIfNeeded()
        ob.presentTutorialIfNeeded()
        #if !APP_STORE_REVIEW
        ob.presentBetaWelcomeIfNeeded()
        #endif
        XCTAssertTrue(nothingShown(ob), "全部済みなら何も出さない")
    }

    // MARK: - β の案内

    #if !APP_STORE_REVIEW
    private func readyForBeta(isBeta: Bool) -> MacOnboarding {
        let ob = onboarding(isBeta: isBeta)
        ob.consent.markPrompted()
        ob.consent.grant()
        ob.tutorial.markTutorialSeen()
        ob.tutorial.markEditHintShown() // 最初のアプリを作った
        return ob
    }

    func testBetaWelcomeAfterFirstAppOnBeta() {
        let ob = readyForBeta(isBeta: true)
        ob.presentBetaWelcomeIfNeeded()
        XCTAssertTrue(ob.showBetaWelcome)
        ob.finishBetaWelcome()
        XCTAssertFalse(ob.showBetaWelcome)
        XCTAssertTrue(ob.betaWelcome.hasSeen, "1 回だけ")
        ob.presentBetaWelcomeIfNeeded()
        XCTAssertFalse(ob.showBetaWelcome)
    }

    func testNoBetaWelcomeOnNonBeta() {
        let ob = readyForBeta(isBeta: false)
        ob.presentBetaWelcomeIfNeeded()
        XCTAssertFalse(ob.showBetaWelcome, "β でない版は β の案内を出さない")
    }

    func testNoBetaWelcomeBeforeFirstApp() {
        let ob = onboarding(isBeta: true)
        ob.consent.markPrompted()
        ob.tutorial.markTutorialSeen()
        ob.presentBetaWelcomeIfNeeded()
        XCTAssertFalse(ob.showBetaWelcome, "最初のアプリの前には出さない")
    }

    func testNoBetaWelcomeOverTutorialOrWhileHolding() {
        let ob = readyForBeta(isBeta: true)
        ob.showTutorial = true
        ob.presentBetaWelcomeIfNeeded()
        XCTAssertFalse(ob.showBetaWelcome, "ほかの案内に重ねない")
        ob.showTutorial = false
        hold.begin(.consent)
        ob.presentBetaWelcomeIfNeeded()
        XCTAssertFalse(ob.showBetaWelcome, "起動時の段が残っている間は出さない")
        hold.end(.consent)
        ob.presentBetaWelcomeIfNeeded()
        XCTAssertTrue(ob.showBetaWelcome)
    }
    #endif
}
