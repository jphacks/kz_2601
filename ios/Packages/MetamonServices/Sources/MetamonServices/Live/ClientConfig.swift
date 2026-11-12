import Foundation

/// appconfig `GET /v1/client-config`(#302・認証不要)。省略されたフィールド = 強制しない。
/// iOS / macOS(#981)の版はそれぞれのフィールドで判定する(`ClientConfig.Platform`)。
public struct ClientConfig: Decodable, Equatable, Sendable {
    public var minIOSBuild: Int?
    public var latestIOSBuild: Int?
    public var iosAppStoreUrl: String?
    /// Mac アプリの CFBundleVersion(backend `MIN_MACOS_BUILD` など・#981)
    public var minMacOSBuild: Int?
    public var latestMacOSBuild: Int?
    public var macAppStoreUrl: String?
    /// アプリ内のお知らせ(#333)。壊れていても nil にするだけで、版の判定は続ける
    public var notice: ServiceNotice?

    /// どの OS の版のフィールドで判定するか
    public enum Platform: Sendable {
        case ios, macos
    }

    public init(minIOSBuild: Int? = nil, latestIOSBuild: Int? = nil, iosAppStoreUrl: String? = nil,
                minMacOSBuild: Int? = nil, latestMacOSBuild: Int? = nil, macAppStoreUrl: String? = nil,
                notice: ServiceNotice? = nil) {
        self.minIOSBuild = minIOSBuild
        self.latestIOSBuild = latestIOSBuild
        self.iosAppStoreUrl = iosAppStoreUrl
        self.minMacOSBuild = minMacOSBuild
        self.latestMacOSBuild = latestMacOSBuild
        self.macAppStoreUrl = macAppStoreUrl
        self.notice = notice
    }

    private enum CodingKeys: String, CodingKey { case minIOSBuild, latestIOSBuild, iosAppStoreUrl, minMacOSBuild, latestMacOSBuild, macAppStoreUrl, notice }

    public init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        minIOSBuild = try c.decodeIfPresent(Int.self, forKey: .minIOSBuild)
        latestIOSBuild = try c.decodeIfPresent(Int.self, forKey: .latestIOSBuild)
        iosAppStoreUrl = try c.decodeIfPresent(String.self, forKey: .iosAppStoreUrl)
        minMacOSBuild = try? c.decodeIfPresent(Int.self, forKey: .minMacOSBuild)
        latestMacOSBuild = try? c.decodeIfPresent(Int.self, forKey: .latestMacOSBuild)
        macAppStoreUrl = try? c.decodeIfPresent(String.self, forKey: .macAppStoreUrl)
        notice = try? c.decodeIfPresent(ServiceNotice.self, forKey: .notice)
    }

    func minBuild(_ platform: Platform) -> Int? { platform == .macos ? minMacOSBuild : minIOSBuild }
    func latestBuild(_ platform: Platform) -> Int? { platform == .macos ? latestMacOSBuild : latestIOSBuild }

    /// サーバーが指定したストアの URL。OS ごとに許すスキームだけ(それ以外は nil = アプリ側の既定)
    func storeURL(_ platform: Platform) -> URL? {
        let (raw, schemes) = platform == .macos ? (macAppStoreUrl, ["https", "macappstore"]) : (iosAppStoreUrl, ["https", "itms-apps"])
        return raw.flatMap(URL.init(string:)).flatMap { schemes.contains($0.scheme ?? "") ? $0 : nil }
    }
}

/// 版の判定(#302)。**fail open**: 設定が取れない・自分の build が読めない時は何もしない(backend が落ちていてもアプリは使える)。
public enum UpdateRequirement: Equatable, Sendable {
    case none
    /// 閉じられるお知らせ(LATEST_IOS_BUILD 未満)
    case recommended(latest: Int)
    /// 閉じられない「アップデートしてください」(MIN_IOS_BUILD 未満)
    case required(minimum: Int)

    /// `platform` の既定は iOS(Mac は `.macos` を渡す・#981)
    public static func evaluate(config: ClientConfig?, build: String, platform: ClientConfig.Platform = .ios) -> UpdateRequirement {
        guard let config, let current = BuildNumber(build) else { return .none }
        if let min = config.minBuild(platform), min > 0, current < BuildNumber(min) { return .required(minimum: min) }
        if let latest = config.latestBuild(platform), latest > 0, current < BuildNumber(latest) { return .recommended(latest: latest) }
        return .none
    }
}

/// CFBundleVersion の比較。`12` / `1.2.3` の数字の並びを左から比べる(足りない桁は 0)。数字でなければ nil。
public struct BuildNumber: Comparable, Sendable {
    let parts: [Int]

    public init?(_ raw: String) {
        let pieces = raw.trimmingCharacters(in: .whitespaces).split(separator: ".", omittingEmptySubsequences: false)
        let parts = pieces.compactMap { Int($0) }
        guard !pieces.isEmpty, parts.count == pieces.count, parts.allSatisfy({ $0 >= 0 }) else { return nil }
        self.parts = parts
    }

    public init(_ value: Int) { parts = [value] }

    public static func < (a: BuildNumber, b: BuildNumber) -> Bool {
        for i in 0..<max(a.parts.count, b.parts.count) {
            let x = i < a.parts.count ? a.parts[i] : 0
            let y = i < b.parts.count ? b.parts[i] : 0
            if x != y { return x < y }
        }
        return false
    }

    public static func == (a: BuildNumber, b: BuildNumber) -> Bool { !(a < b) && !(b < a) }
}

public protocol ClientConfigClient: Sendable {
    func fetch() async throws -> ClientConfig
}

/// `GET {appconfig}/v1/client-config`。トークンは付けない(サインイン前にも呼ぶ)。
public struct HTTPClientConfigClient: ClientConfigClient {
    public let baseURL: URL
    public var timeout: TimeInterval

    public init(baseURL: URL, timeout: TimeInterval = 8) {
        self.baseURL = baseURL
        self.timeout = timeout
    }

    public func fetch() async throws -> ClientConfig {
        let data = try await HTTP.send("GET", baseURL: baseURL, path: "v1/client-config", body: nil, token: NoAuthTokenProvider(), timeout: timeout)
        return try JSONDecoder().decode(ClientConfig.self, from: data)
    }
}

/// 起動時・前面に戻った時の確認。失敗はすべて `.none`(fail open)。
public struct UpdateChecker: Sendable {
    public var client: (any ClientConfigClient)?
    public var build: String
    /// どの OS の版で判定するか(Mac は `.macos`・#981)
    public var platform: ClientConfig.Platform

    public init(client: (any ClientConfigClient)?, build: String = ClientInfo.current.build, platform: ClientConfig.Platform = .ios) {
        self.client = client
        self.build = build
        self.platform = platform
    }

    public struct Result: Equatable, Sendable {
        public var requirement: UpdateRequirement
        /// サーバーが指定した App Store の URL(無ければ nil = アプリ側の既定)
        public var appStoreURL: URL?
        /// この build に出すお知らせ(#333)。閉じたかどうかは見ない(`NoticeDismissals`)
        public var notice: ServiceNotice?

        public init(requirement: UpdateRequirement, appStoreURL: URL? = nil, notice: ServiceNotice? = nil) {
            self.requirement = requirement
            self.appStoreURL = appStoreURL
            self.notice = notice
        }
    }

    public func check() async -> Result {
        guard let client, let config = try? await client.fetch() else { return Result(requirement: .none) }
        let notice = config.notice.flatMap { $0.isDisplayable && $0.applies(toBuild: build) ? $0 : nil }
        return Result(requirement: UpdateRequirement.evaluate(config: config, build: build, platform: platform),
                      appStoreURL: config.storeURL(platform), notice: notice)
    }
}
