import Foundation

/// 音声モデル(WhisperKit)の初回ダウンロードの前の確認(#246・App Store 4.2.3(ii))。
/// 端末にモデルが無い時だけ、サイズを示して確認する。一度「ダウンロード」を選んだら、失敗後の再試行では聞き直さない。
public enum ModelDownloadConsent {
    static let approvedKey = "metamon.modelDownload.approved"

    public enum Decision: Equatable, Sendable {
        /// 既にある / 承認済み → そのまま prepare
        case prepare
        /// サイズを示して確認する(prepare はまだ呼ばない)
        case ask
    }

    public static func decide(needsDownload: Bool, defaults: UserDefaults = .standard) -> Decision {
        guard needsDownload else { return .prepare }
        return isApproved(defaults) ? .prepare : .ask
    }

    public static func isApproved(_ defaults: UserDefaults = .standard) -> Bool {
        defaults.bool(forKey: approvedKey)
    }

    public static func approve(_ defaults: UserDefaults = .standard) {
        defaults.set(true, forKey: approvedKey)
    }

    /// モデル名からおおよそのダウンロードサイズ(MB)。argmaxinc/whisperkit-coreml の実サイズの目安。
    public static func approximateMegabytes(model: String) -> Int {
        let m = model.lowercased()
        if m.contains("tiny") { return 75 }
        if m.contains("base") { return 150 }
        if m.contains("small") { return 500 }
        if m.contains("medium") { return 1_500 }
        if m.contains("large") { return m.contains("626mb") ? 626 : 1_600 }
        return 500
    }
}
