import Foundation
import FirebaseCrashlytics

/// Firebase Crashlytics 実装(#296)。dSYM の送り方は docs/ios/crash-reporting.md。
public struct FirebaseCrashReporter: CrashReporter {
    /// Firebase 未初期化なら nil(呼び出し側が Noop に落とす)。
    public init?() {
        guard FirebaseBootstrap.isConfigured else { return nil }
    }

    public func setCollectionEnabled(_ enabled: Bool) {
        let crashlytics = Crashlytics.crashlytics()
        crashlytics.setCrashlyticsCollectionEnabled(enabled) // SDK が保存するので次の起動の最初から効く
        if !enabled { crashlytics.deleteUnsentReports() }     // オフにする前に溜まった分も送らない
    }

    public func setUserID(_ uid: String?) {
        Crashlytics.crashlytics().setUserID(uid)
    }
}
