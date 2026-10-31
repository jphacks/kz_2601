import Foundation
import Observation

/// クラッシュ報告の送り先(#296)。わざと「ログ」「任意のキー」を持たせない=話した内容・記録・DSL を送る口を作らない。
/// 送るのは SDK が自動で集めるクラッシュの情報(スタック・端末・OS・アプリの版)と、本人を区別する uid だけ。
public protocol CrashReporter: Sendable {
    func setCollectionEnabled(_ enabled: Bool)
    func setUserID(_ uid: String?)
}

/// Firebase が無い環境(plist 未同期・テスト)用。何もしない。
public struct NoopCrashReporter: CrashReporter {
    public init() {}
    public func setCollectionEnabled(_ enabled: Bool) {}
    public func setUserID(_ uid: String?) {}
}

/// 設定の「クラッシュ報告を送る」(#296)。既定はオン(プライバシーポリシー 1.8 に明記)。
/// 端末の UserDefaults に保存し、変えたらすぐ送り先(Crashlytics)に反映する。
@MainActor
@Observable
public final class CrashReportingPreference {
    nonisolated static let enabledKey = "metamon.crashReporting.enabled"

    public static let shared = CrashReportingPreference(defaults: .standard)

    public private(set) var isEnabled: Bool

    @ObservationIgnored private let defaults: UserDefaults
    @ObservationIgnored private var reporter: any CrashReporter = NoopCrashReporter()
    @ObservationIgnored private var uid: String?

    public init(defaults: UserDefaults) {
        self.defaults = defaults
        isEnabled = defaults.object(forKey: Self.enabledKey) as? Bool ?? true
    }

    /// 起動時に 1 回: 送り先をつなぎ、保存済みの設定を反映する。
    public func activate(reporter: any CrashReporter) {
        self.reporter = reporter
        reporter.setCollectionEnabled(isEnabled)
        reporter.setUserID(isEnabled ? uid : nil)
    }

    public func setEnabled(_ enabled: Bool) {
        defaults.set(enabled, forKey: Self.enabledKey)
        isEnabled = enabled
        reporter.setCollectionEnabled(enabled)
        reporter.setUserID(enabled ? uid : nil)
    }

    /// サインイン中の uid(サインアウトで nil)。オフの間は送り先に渡さない。
    public func updateUser(uid: String?) {
        self.uid = uid
        reporter.setUserID(isEnabled ? uid : nil)
    }
}
