import Foundation

/// Mac の接続先(#2247・判定の本体は ios/Shared/Debug/DebugMenuPolicy.swift)。読み方は MacServices と同じ
extension DebugMenuPolicy {
    static var currentAIURL: URL? { MacServices.url(env: "METAMON_AI_BASE_URL", plist: "MetamonAIBaseURL") }
    static var currentAppConfigURL: URL? { MacServices.url(env: "METAMON_APPCONFIG_BASE_URL", plist: "MetamonAppConfigBaseURL") }
}
