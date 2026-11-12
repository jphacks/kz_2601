import SwiftUI
import Observation
import MetamonServices

/// Mac の初回の案内の順番(#971)。iOS と同じ時点・同じ保存先で出す:
/// 同意の説明(#246・版が上がった時も 1 回)→ チュートリアル(#274)→ β の案内(#313・最初のアプリの後・TestFlight の版だけ)。
/// iOS の `ConsentHost` / `TutorialHost` / `BetaWelcomeHost` の Mac 版。判定は ios/Shared と MetamonServices の保存先に任せる。
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

    #if !APP_STORE_REVIEW
    /// β の版で、チュートリアルを見て最初のアプリを作った後に 1 回だけ。最初のアプリは iOS と同じ印(`hasShownEditHint`)で見る。
    func presentBetaWelcomeIfNeeded() {
        guard !showBetaWelcome, !showConsent, !showTutorial,
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
            .sheet(isPresented: $onboarding.showConsent) { MacAIConsentSheet() }
            .sheet(isPresented: Binding(get: { onboarding.showTutorial },
                                        set: { if !$0 { onboarding.finishTutorial() } })) {
                MacTutorialView { onboarding.finishTutorial() }
            }
            #if !APP_STORE_REVIEW
            .sheet(isPresented: Binding(get: { onboarding.showBetaWelcome },
                                        set: { if !$0 { onboarding.finishBetaWelcome() } })) {
                MacBetaWelcomeView { onboarding.finishBetaWelcome() }
            }
            #endif
            // 中のアプリのウインドウが「直し方の例」を閉じた時に β の案内を出せるよう渡す(#1215)
            .environment(onboarding)
            .onChange(of: onboarding.showConsent) { _, showing in
                // 同意の説明が閉じたら、閉じるアニメーションを待ってからチュートリアル(重なると表示されない)
                guard !showing else { return }
                Task { @MainActor in
                    try? await Task.sleep(for: .milliseconds(700))
                    onboarding.presentTutorialIfNeeded()
                }
            }
            .task {
                // iOS と同じ待ち: 同意 0.5 秒 → (済みなら)チュートリアル 0.8 秒 → β の案内 1.2 秒
                try? await Task.sleep(for: .milliseconds(500))
                onboarding.presentConsentIfNeeded()
                try? await Task.sleep(for: .milliseconds(300))
                onboarding.presentTutorialIfNeeded()
                #if !APP_STORE_REVIEW
                try? await Task.sleep(for: .milliseconds(400))
                onboarding.presentBetaWelcomeIfNeeded()
                #endif
            }
    }
}

extension View {
    func macOnboarding() -> some View { modifier(MacOnboardingHost()) }
}
