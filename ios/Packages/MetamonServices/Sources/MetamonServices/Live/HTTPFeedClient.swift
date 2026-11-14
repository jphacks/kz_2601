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
        // POST: diet の検索語などを URL に載せない(GET は古いビルドのためにサーバーに残っているだけ)
        let data = try await HTTP.send("POST", baseURL: baseURL, path: query.path, body: query, token: token, timeout: 20)
        return try JSONDecoder().decode(FeedResult.self, from: data)
    }
}
#endif
