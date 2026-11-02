// 審査用ビルドでは外す(β の案内ごと・#313・docs/ios/review-build.md)。
#if !APP_STORE_REVIEW
import Foundation
import Observation

/// β の初回の案内(#313)を出すかの判定と「見たか」の保存。文言は `BetaWelcomeView`(審査用の版には入らない)。
/// 表示の順序は 同意(#246)→ チュートリアル(#274)→ β の案内 → 音声モデルのダウンロード確認。
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

    /// 初回に自動で出すか。β の版で、チュートリアルを見終えていて、まだ見ていない時だけ。
    nonisolated static func shouldAutoPresent(isBeta: Bool, hasSeen: Bool, tutorialSeen: Bool) -> Bool {
        isBeta && tutorialSeen && !hasSeen
    }

    func shouldAutoPresent(tutorialSeen: Bool) -> Bool {
        Self.shouldAutoPresent(isBeta: isBeta, hasSeen: hasSeen, tutorialSeen: tutorialSeen)
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
    /// β の案内をまだ出していないか(音声モデルのダウンロード確認を後に回すのに使う)。
    var isBetaWelcomePending: Bool {
        betaWelcome?.shouldAutoPresent(tutorialSeen: hasSeenTutorial) ?? false
    }

    /// 設定に「β 版のご案内」を出すか。
    var canOpenBetaWelcome: Bool { betaWelcome?.isBeta ?? false }

    /// 起動時・チュートリアルが閉じた時に呼ぶ。
    func presentBetaWelcomeIfNeeded() {
        guard !showBetaWelcome, !showTutorial, !showAIConsent, isBetaWelcomePending else { return }
        print("[Metamon] beta welcome: presenting")
        showBetaWelcome = true
    }

    /// 設定の「β 版のご案内」。シートを閉じきってから出す(シートの同時表示は SwiftUI が無視する)。
    func openBetaWelcome() {
        guard canOpenBetaWelcome else { return }
        showVoiceSheet = false
        Task { @MainActor [weak self] in
            try? await Task.sleep(for: .milliseconds(600))
            self?.showBetaWelcome = true
        }
    }

    /// 閉じ方を問わず見た扱い(次から自動では出さない)。
    func finishBetaWelcome() {
        betaWelcome?.markSeen()
        showBetaWelcome = false
    }
}
#endif
