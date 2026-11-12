import SwiftUI
import Observation
import MetamonServices

/// Mac の初回の案内の順番(#971)。iOS と同じ時点・同じ保存先で出す:
/// 同意の説明(#246・版が上がった時も 1 回)→ チュートリアル(#274)→ β の案内(#313・最初のアプリの後・TestFlight の版だけ)。
/// iOS の `ConsentHost` / `TutorialHost` / `BetaWelcomeHost` の Mac 版。判定は ios/Shared と MetamonServices の保存先に任せる。
/// 起動時の順番(#1885)は iOS と同じ `LaunchConsentHold`: 同意 → チュートリアル → シェアリンク → Intent を、前の画面が閉じきってから 1 つずつ。
/// 出さない段はすぐ次へ。β の案内と更新のお知らせは全段が終わってから。
@MainActor
@Observable
final class MacOnboarding {
    var showConsent = false
    var showTutorial = false
    var showBetaWelcome = false

    @ObservationIgnored let consent: AIConsentStore
    @ObservationIgnored let tutorial: FirstRunTutorialStore
    #if !APP_STORE_REVIEW
    @ObservationIgnored let betaWelcome: BetaWelcomeStore
    #endif
    /// 起動時の画面の順番待ち(iOS と共有・#1885)。テストでは別の物を渡す
    @ObservationIgnored var hold: LaunchConsentHold = .shared
    /// Mac にスプラッシュは無い(出ないまま)。iOS と同じ形で待つ
    @ObservationIgnored var splash: SplashPresence = .shared

    #if APP_STORE_REVIEW
    init(consent: AIConsentStore = .shared, tutorial: FirstRunTutorialStore = .shared) {
        self.consent = consent
        self.tutorial = tutorial
    }
    #else
    init(consent: AIConsentStore = .shared, tutorial: FirstRunTutorialStore = .shared, betaWelcome: BetaWelcomeStore = .shared) {
        self.consent = consent
        self.tutorial = tutorial
        self.betaWelcome = betaWelcome
    }
    #endif

    /// 起動時: まだ説明を出していなければ 1 回だけ(iOS と同じ `shouldPromptOnLaunch`)。
    func presentConsentIfNeeded() {
        guard !showConsent, !showTutorial, !showBetaWelcome, consent.shouldPromptOnLaunch else { return }
        showConsent = true
    }

    /// 同意の説明を出し終えていて、まだ見ていなければ出す(iOS の `presentTutorialIfNeeded`)。
    func presentTutorialIfNeeded() {
        guard !showTutorial, !showConsent, !showBetaWelcome,
              tutorial.shouldAutoPresent(consentPrompted: consent.isGranted || consent.hasBeenPrompted) else { return }
        showTutorial = true
    }

    func finishTutorial() {
        tutorial.markTutorialSeen()
        showTutorial = false
    }

    // MARK: - 起動時の順番(#1885・iOS の AppRuntime+Consent / +Tutorial と同じ)

    /// 起動時(`MetamonMacApp`): 同意の画面・チュートリアルを出すなら、閉じるまで後ろ(シェアリンク・Intent)を待たせる。
    static func beginLaunchHold(consent: AIConsentStore = .shared, tutorial: FirstRunTutorialStore = .shared,
                                hold: LaunchConsentHold = .shared) {
        if consent.shouldPromptOnLaunch { hold.begin(.consent) }
        if !tutorial.hasSeenTutorial { hold.begin(.tutorial) }
        hold.finishLaunchIfIdle() // 待たせる段が無ければ、後から届いたリンクは待たせない
    }

    /// 起動時の同意の画面。出さない(済み)・出す前に取り消された時は、待たせていた段をすぐ放す。
    func presentLaunchConsent(delay: Duration = .milliseconds(500)) async {
        guard consent.shouldPromptOnLaunch else { hold.end(.consent); return }
        await splash.waitUntilHidden()
        try? await Task.sleep(for: delay)
        guard !Task.isCancelled else { hold.end(.consent); return }
        presentConsentIfNeeded()
        if !showConsent { hold.end(.consent) }
    }

    /// 同意の画面が閉じきった(同意・「あとで」どちらでも)。次の段へ
    func launchConsentDismissed() { hold.end(.consent) }

    /// 起動時のチュートリアルの段。同意の画面が閉じきってから、閉じるアニメーションの分だけ待って出す。
    /// 出さない(見た・同意の説明が済んでいない)・取り消された時はすぐ次の段へ。出した時は `tutorialDismissed()` で次へ。
    func runLaunchTutorialStep(settle: Duration = .milliseconds(700)) async {
        await splash.waitUntilHidden()
        await hold.waitUntilReleased(before: .tutorial)
        try? await Task.sleep(for: settle)
        guard !Task.isCancelled else { hold.end(.tutorial); return }
        presentTutorialIfNeeded()
        if !showTutorial { hold.end(.tutorial) }
    }

    /// チュートリアルが閉じきった(シートの onDismiss)。待たせていたシェアリンク・Intent を出す
    func tutorialDismissed() { hold.end(.tutorial) }

    /// 起動時の段がまだ残っているか(β の案内・更新のお知らせを出さない)
    var isLaunchHolding: Bool { splash.isShowing || hold.isHolding }

    #if !APP_STORE_REVIEW
    /// β の版で、チュートリアルを見て最初のアプリを作った後に 1 回だけ。最初のアプリは iOS と同じ印(`hasShownEditHint`)で見る。
    /// 起動時の段(同意・チュートリアル・シェアリンク)が全部閉じてから(#1885)。
    func presentBetaWelcomeIfNeeded() {
        guard !isLaunchHolding, !showBetaWelcome, !showConsent, !showTutorial,
              betaWelcome.shouldAutoPresent(tutorialSeen: tutorial.hasSeenTutorial, firstAppMade: tutorial.hasShownEditHint) else { return }
        showBetaWelcome = true
    }

    func finishBetaWelcome() {
        betaWelcome.markSeen()
        showBetaWelcome = false
    }
    #endif
}

/// 本体のウィンドウに付ける: 初回の同意・チュートリアル・β の案内をシートで出す(#971)。
struct MacOnboardingHost: ViewModifier {
    @State private var onboarding = MacOnboarding()

    func body(content: Content) -> some View {
        @Bindable var onboarding = onboarding
        content
            // 閉じきった時(onDismiss)に次の段へ。出た時(onAppear)からは時間で放さない(#1885)
            .sheet(isPresented: $onboarding.showConsent, onDismiss: { onboarding.launchConsentDismissed() }) {
                MacAIConsentSheet()
                    .onAppear { onboarding.hold.markShown(.consent) }
            }
            .sheet(isPresented: Binding(get: { onboarding.showTutorial },
                                        set: { if !$0 { onboarding.finishTutorial() } }),
                   onDismiss: { onboarding.tutorialDismissed() }) {
                MacTutorialView { onboarding.finishTutorial() }
                    .onAppear { onboarding.hold.markShown(.tutorial) }
            }
            #if !APP_STORE_REVIEW
            .sheet(isPresented: Binding(get: { onboarding.showBetaWelcome },
                                        set: { if !$0 { onboarding.finishBetaWelcome() } })) {
                MacBetaWelcomeView { onboarding.finishBetaWelcome() }
            }
            #endif
            // 中のアプリのウインドウが「直し方の例」を閉じた時に β の案内を出せるよう渡す(#1215)
            .environment(onboarding)
            .task {
                // 起動時の順番(#1885・iOS と同じ): 同意 → (閉じきってから)チュートリアル → シェアリンク・Intent → β の案内
                onboarding.hold.hostsAppeared() // 出せない段を時間で放す見張りはここから
                await onboarding.presentLaunchConsent()
                await onboarding.runLaunchTutorialStep()
                #if !APP_STORE_REVIEW
                await onboarding.hold.waitUntilReleased()
                try? await Task.sleep(for: .milliseconds(700)) // 前のシートが閉じるのを待つ(重なると表示されない)
                guard !Task.isCancelled else { return }
                onboarding.presentBetaWelcomeIfNeeded()
                #endif
            }
    }
}

extension View {
    func macOnboarding() -> some View { modifier(MacOnboardingHost()) }
}
