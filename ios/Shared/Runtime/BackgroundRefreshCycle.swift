import Foundation

/// 裏での定期の取り直しの共有の決まり(iOS の `BGAppRefreshTask`・#38 / Mac の `NSBackgroundActivityScheduler`・#2253)。
/// 飛ばす条件と順番(未送信を流す → 取り直す)はここだけで決め、両方の OS がこの `perform` を呼ぶ(写しを作らない)。
enum BackgroundRefreshCycle {
    /// OS への希望の間隔(iOS は「これより早くは起こさない」・Mac はスケジューラの間隔)
    static let interval: TimeInterval = 15 * 60

    enum Outcome: Equatable {
        /// サインアウト中(トークン無しで送らない・未送信は次のサインインまで端末に残す)
        case skippedSignedOut
        /// オフライン(送れず取れないので起こしても無駄)
        case skippedOffline
        case refreshed
    }

    /// 飛ばす理由。nil なら走らせる
    static func skipReason(signedIn: Bool, online: Bool) -> Outcome? {
        if !signedIn { return .skippedSignedOut }
        if !online { return .skippedOffline }
        return nil
    }

    /// 未送信のキューを流してから、一覧・開いているアプリを取り直す。どちらかが失敗しても残りは続ける(各処理の中で握る)
    @MainActor
    @discardableResult
    static func perform(reason: String, signedIn: Bool, online: Bool,
                        drain: () async -> Void, refetch: () async -> Void) async -> Outcome {
        if let skip = skipReason(signedIn: signedIn, online: online) {
            print("[Metamon] background(\(reason)): \(skip == .skippedSignedOut ? "signed out" : "offline"), skipped")
            return skip
        }
        await drain()
        await refetch()
        return .refreshed
    }
}
