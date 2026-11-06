// 審査用ビルド(APP_STORE_REVIEW)は外部の公開データ(feed・#348)を外す。
#if !APP_STORE_REVIEW
import Foundation
import MetamonDSL
import MetamonRenderer
import MetamonServices

extension AppRuntime: FeedHost {
    /// appconfig の feeds API を読む(live の時だけ)。同じ問い合わせは `FeedCache` が 10 分覚える。
    func feed(_ query: FeedQuery, force: Bool) async throws -> FeedResult {
        guard let client = services.feeds else { throw URLError(.unsupportedURL) }
        return try await FeedCache.shared.load(query, force: force, client: client)
    }
}
#endif
