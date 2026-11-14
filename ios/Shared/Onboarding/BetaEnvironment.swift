// 審査用ビルドでは外す(β の案内ごと・#313・docs/ios/review-build.md)。
#if !APP_STORE_REVIEW
import Foundation

/// β(TestFlight)の版かどうかの判定(#313)。判定はここだけに置く。
///
/// TestFlight と App Store は同じバイナリなので、コンパイル時のフラグでは見分けられない。
/// 実行時にレシートの場所で見分ける(TestFlight は `sandboxReceipt`、App Store は `receipt`)。
/// 審査用の版(`APP_STORE_REVIEW`・#245)は常に β ではない。
enum BetaEnvironment {
    /// レシートが sandbox のものか(TestFlight・Xcode から入れた版)。
    static func isSandboxReceipt(_ url: URL?) -> Bool {
        url?.lastPathComponent == "sandboxReceipt"
    }

    /// TestFlight(か Xcode から入れた版)で動いているか。
    static var isTestFlight: Bool { isSandboxReceipt(Bundle.main.appStoreReceiptURL) }

    static var isReviewBuild: Bool {
        #if APP_STORE_REVIEW
        true
        #else
        false
        #endif
    }

    static var isDebugBuild: Bool {
        #if DEBUG
        true
        #else
        false
        #endif
    }

    /// β の案内を出す版か。
    /// - 審査用の版: 出さない
    /// - Debug: Xcode やシミュレータでもレシートが sandbox になるので、`METAMON_BETA=1` を付けた時だけ
    /// - それ以外(Release): TestFlight のときだけ
    static func isBeta(isTestFlight: Bool, isReviewBuild: Bool, isDebugBuild: Bool, forceBeta: Bool) -> Bool {
        if isReviewBuild { return false }
        if isDebugBuild { return forceBeta }
        return isTestFlight
    }

    static func current(environment: [String: String] = ProcessInfo.processInfo.environment) -> Bool {
        isBeta(isTestFlight: isTestFlight, isReviewBuild: isReviewBuild, isDebugBuild: isDebugBuild,
               forceBeta: environment["METAMON_BETA"] == "1")
    }
}
#endif
