// 審査用ビルド(APP_STORE_REVIEW)は外部の公開データ(feed・#348)を外す。
#if !APP_STORE_REVIEW
import Foundation
import MetamonDSL

/// 外部の公開データ(#348・`POST /v1/feeds/{provider}`)。端末は appconfig にだけ聞き、外部へ直接つながらない。
public protocol FeedClient: Sendable {
    func feed(_ query: FeedQuery) async throws -> FeedResult
}

/// live の時だけ差し込む(モックとインメモリでは nil = 部品は「読み込めませんでした」)。
public enum FeedServiceKey: ServiceKey {
    public static var defaultValue: (any FeedClient)? { nil }
}

extension Services {
    public var feeds: (any FeedClient)? {
        get { self[FeedServiceKey.self] }
        set { self[FeedServiceKey.self] = newValue }
    }
}

/// 同じ問い合わせを 10 分覚える(画面を行き来するたびに取りに行かない)。サーバーも数分〜数時間キャッシュする。
@MainActor
public final class FeedCache {
    public static let shared = FeedCache()
    public static let ttl: TimeInterval = 600

    private var entries: [FeedQuery: (result: FeedResult, at: Date)] = [:]
    /// `reset` のたびに進める。取りに行っている間にサインアウトされたら、前の人の結果を書き戻さない
    private var generation = 0
    private let now: () -> Date

    public init(now: @escaping () -> Date = Date.init) { self.now = now }

    public func load(_ query: FeedQuery, force: Bool, client: any FeedClient) async throws -> FeedResult {
        if !force, let e = entries[query], now().timeIntervalSince(e.at) < Self.ttl { return e.result }
        let started = generation
        let result = try await client.feed(query)
        if generation == started { entries[query] = (result, now()) }
        return result
    }

    /// 覚えている結果(古くなっていても返す・取りに行かない)。
    public func cached(_ query: FeedQuery) -> FeedResult? { entries[query]?.result }

    /// サインアウト・テスト用。
    public func reset() {
        entries = [:]
        generation += 1
    }
}
#endif
