// 審査用ビルドでは外す(β の案内ごと・#313・docs/ios/review-build.md)。
#if !APP_STORE_REVIEW
import Foundation
import Observation

/// β の初回の案内(#313)を出すかの判定と「見たか」の保存。文言は `BetaWelcomeView`(審査用の版には入らない)。
/// 初回の関門を減らすため、最初のアプリの後に出す(UX 点検 H1・2026-11-06 オーナーの決定 U6)。
/// 最初のアプリの直後の「直し方の例」を閉じた時(`BetaWelcomeHost`)か、次の起動で 1 回だけ。iOS と Mac で共有(#971)。
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
#endif
