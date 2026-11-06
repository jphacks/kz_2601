// 審査用ビルドでは外す(β の案内ごと・#313・docs/ios/review-build.md)。
#if !APP_STORE_REVIEW
import Foundation
import Observation

/// β の初回の案内(#313)を出すかの判定と「見たか」の保存。文言は `BetaWelcomeView`(審査用の版には入らない)。
/// 初回の関門を減らすため、最初のアプリの後に出す(UX 点検 H1・2026-11-06 オーナーの決定 U6)。
/// 最初のアプリの直後の「直し方の例」を閉じた時(`BetaWelcomeHost`)か、次の起動で 1 回だけ。
@MainActor
@Observable
final class BetaWelcomeStore {
    /// 案内の中身を大きく変えたら版を上げて、もう一度出す。
    nonisolated static let seenKey = "metamon.betaWelcome.seen.v1"

    static let shared = BetaWelcomeStore(defaults: .standard, isBeta: BetaEnvironment.current())

    let isBeta: Bool
    private(set) var hasSeen: Bool
    @ObservationIgnored private let defaults: UserDefaults

    init(defaults: UserDefaults, isBeta: Bool) {
        self.defaults = defaults
        self.isBeta = isBeta
        hasSeen = defaults.bool(forKey: Self.seenKey)
    }

    /// 自動で出すか。β の版で、チュートリアルを見終えて最初のアプリを作っていて、まだ見ていない時だけ。
    nonisolated static func shouldAutoPresent(isBeta: Bool, hasSeen: Bool, tutorialSeen: Bool, firstAppMade: Bool) -> Bool {
        isBeta && tutorialSeen && firstAppMade && !hasSeen
    }

    func shouldAutoPresent(tutorialSeen: Bool, firstAppMade: Bool) -> Bool {
        Self.shouldAutoPresent(isBeta: isBeta, hasSeen: hasSeen, tutorialSeen: tutorialSeen, firstAppMade: firstAppMade)
    }

    func markSeen() {
        defaults.set(true, forKey: Self.seenKey)
        hasSeen = true
    }

    /// UI テスト用(DEBUG のみ): `METAMON_BETA_WELCOME=reset` で初回、`=seen` で見た状態で起動する。
    func applyLaunchOverride(environment: [String: String] = ProcessInfo.processInfo.environment) {
        #if DEBUG
        switch environment["METAMON_BETA_WELCOME"] {
        case "reset":
            defaults.removeObject(forKey: Self.seenKey)
            hasSeen = false
        case "seen":
            markSeen()
        default:
            break
        }
        #endif
    }
}

extension AppRuntime {
    /// β の案内を自動で出す時か(最初のアプリの後・まだ見ていない)。
    var isBetaWelcomePending: Bool {
        betaWelcome?.shouldAutoPresent(tutorialSeen: hasSeenTutorial, firstAppMade: hasMadeFirstApp) ?? false
    }

    /// 設定に「β 版のご案内」を出すか。
    var canOpenBetaWelcome: Bool { betaWelcome?.isBeta ?? false }

    /// 起動時・最初のアプリの直後の「直し方の例」が閉じた時に呼ぶ。作っている途中・他のシートが出ている間は出さない。
    func presentBetaWelcomeIfNeeded() {
        guard !showBetaWelcome, !showTutorial, !showAIConsent, !showVoiceSheet, !showFirstEditHint, !showLinkPrompt,
              morphJob == nil, isBetaWelcomePending else { return }
        print("[Metamon] beta welcome: presenting")
        showBetaWelcome = true
    }

    /// 設定の「β 版のご案内」。シートを閉じきってから出す(シートの同時表示は SwiftUI が無視する)。
    func openBetaWelcome() {
        guard canOpenBetaWelcome else { return }
        showVoiceSheet = false
        showAppSwitcher = false // 変身一覧の歯車から開いた設定(H10)
        Task { @MainActor [weak self] in
            try? await Task.sleep(for: .milliseconds(600))
            self?.showBetaWelcome = true
        }
    }

    /// 「試してほしい依頼」: 入力欄に入れて閉じる(送らない)。アプリを開いている時(最初のアプリの後に出すので通常こちら)は
    /// 入力欄のある音声シートを、案内が閉じきってから開く。
    func useBetaWelcomeExample(_ text: String) {
        composerDraft = text
        finishBetaWelcome()
        guard !dsl.isVoiceOnly else { return }
        Task { @MainActor [weak self] in
            try? await Task.sleep(for: .milliseconds(600))
            self?.showVoiceSheet = true
        }
    }

    /// 閉じ方を問わず見た扱い(次から自動では出さない)。
    func finishBetaWelcome() {
        betaWelcome?.markSeen()
        showBetaWelcome = false
    }
}
#endif
