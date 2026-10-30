import Foundation

/// appconfig `GET /v1/client-config`(#302・認証不要)。省略されたフィールド = 強制しない。
public struct ClientConfig: Decodable, Equatable, Sendable {
    public var minIOSBuild: Int?
    public var latestIOSBuild: Int?
    public var iosAppStoreUrl: String?

    public init(minIOSBuild: Int? = nil, latestIOSBuild: Int? = nil, iosAppStoreUrl: String? = nil) {
        self.minIOSBuild = minIOSBuild
        self.latestIOSBuild = latestIOSBuild
        self.iosAppStoreUrl = iosAppStoreUrl
    }
}

/// 版の判定(#302)。**fail open**: 設定が取れない・自分の build が読めない時は何もしない(backend が落ちていてもアプリは使える)。
public enum UpdateRequirement: Equatable, Sendable {
    case none
    /// 閉じられるお知らせ(LATEST_IOS_BUILD 未満)
    case recommended(latest: Int)
    /// 閉じられない「アップデートしてください」(MIN_IOS_BUILD 未満)
    case required(minimum: Int)

    public static func evaluate(config: ClientConfig?, build: String) -> UpdateRequirement {
        guard let config, let current = BuildNumber(build) else { return .none }
        if let min = config.minIOSBuild, min > 0, current < BuildNumber(min) { return .required(minimum: min) }
        if let latest = config.latestIOSBuild, latest > 0, current < BuildNumber(latest) { return .recommended(latest: latest) }
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

    public init(client: (any ClientConfigClient)?, build: String = ClientInfo.current.build) {
        self.client = client
        self.build = build
    }

    public struct Result: Equatable, Sendable {
        public var requirement: UpdateRequirement
        /// サーバーが指定した App Store の URL(無ければ nil = アプリ側の既定)
        public var appStoreURL: URL?
    }

    public func check() async -> Result {
        guard let client, let config = try? await client.fetch() else { return Result(requirement: .none, appStoreURL: nil) }
        let url = config.iosAppStoreUrl.flatMap(URL.init(string:)).flatMap { ["https", "itms-apps"].contains($0.scheme ?? "") ? $0 : nil }
        return Result(requirement: UpdateRequirement.evaluate(config: config, build: build), appStoreURL: url)
    }
}
