// 審査用ビルド(APP_STORE_REVIEW)は外部の公開データ(feed・#348)を外す(iOS と同じ)。
#if !APP_STORE_REVIEW
import Foundation
import MetamonDSL
import MetamonRenderer
import MetamonServices

/// feed の部品(#961・#1288)。iOS の `AppRuntime+Feeds.swift` と同じく appconfig の `POST /v1/feeds/{provider}` を読む。
/// 同じ問い合わせは `FeedCache`(MetamonServices・iOS と共有)が 10 分覚え、取れなかった時は部品が覚えている前の内容を出す(stale)。
/// サーバーにつながっていない時(モック・appconfig の URL 無し)は `feedClient` が nil で、部品は「読み込めませんでした」(iOS のモックと同じ)。
extension MacAppRuntime: FeedHost {
    func feed(_ query: FeedQuery, force: Bool) async throws -> FeedResult {
        guard let client = feedClient else { throw URLError(.unsupportedURL) }
        return try await FeedCache.shared.load(query, force: force, client: client)
    }

    func cachedFeed(_ query: FeedQuery) -> FeedResult? { FeedCache.shared.cached(query) }
}
#endif
