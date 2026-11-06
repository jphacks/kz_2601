// 審査用ビルド(APP_STORE_REVIEW)ではバイナリから外す(#348)。
#if !APP_STORE_REVIEW
import Foundation
import MetamonDSL

/// appconfig の feeds API(#348)。
public struct HTTPFeedClient: FeedClient {
    public let baseURL: URL
    public let token: any TokenProvider

    public init(baseURL: URL, token: any TokenProvider) {
        self.baseURL = baseURL
        self.token = token
    }

    public func feed(_ query: FeedQuery) async throws -> FeedResult {
        let data = try await HTTP.send("GET", baseURL: baseURL, path: query.path, body: nil, token: token, timeout: 20)
        return try JSONDecoder().decode(FeedResult.self, from: data)
    }
}
#endif
