import Foundation
import MetamonDSL

/// この iOS の版(#302)。backend への全リクエストにヘッダで付ける(どの版がまだ使われているかをログで見て、最低対応版を上げる判断に使う)。
/// 部品の絞り込みはヘッダではなく generate / chat の body `allowedComponents`(`ClientCapabilities`)で行う。docs/dsl/README.md §5
public struct ClientInfo: Equatable, Sendable {
    /// CFBundleShortVersionString(例 1.0.0)
    public var version: String
    /// CFBundleVersion(例 12)。最低対応版の比較はこちら
    public var build: String
    public var dslVersion: Int

    public init(version: String, build: String, dslVersion: Int = ClientCapabilities.supportedSchemaVersion) {
        self.version = version
        self.build = build
        self.dslVersion = dslVersion
    }

    public static let current: ClientInfo = {
        let info = Bundle.main.infoDictionary ?? [:]
        return ClientInfo(version: info["CFBundleShortVersionString"] as? String ?? "0",
                          build: info["CFBundleVersion"] as? String ?? "0")
    }()

    public var headers: [String: String] {
        [
            "X-Client-Platform": "ios",
            "X-Client-Version": version,
            "X-Client-Build": build,
            "X-Client-DSL-Version": String(dslVersion),
        ]
    }

    func apply(to request: inout URLRequest) {
        for (k, v) in headers { request.setValue(v, forHTTPHeaderField: k) }
    }
}
