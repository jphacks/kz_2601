import Foundation

/// iOS の接続先(#2247・判定の本体は ios/Shared/Debug/DebugMenuPolicy.swift)
extension DebugMenuPolicy {
    static var currentAIURL: URL? { BuildConfig.aiBaseURL }
    static var currentAppConfigURL: URL? { BuildConfig.appConfigBaseURL }
}
