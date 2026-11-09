// 審査用ビルド(APP_STORE_REVIEW)は外部の公開データ(feed・#348)を外す。
#if !APP_STORE_REVIEW
import MetamonDSL

/// 外部の公開データ(#348)。appconfig の `GET /v1/feeds/{provider}` を読む。端末は外部へ直接つながらない。
/// 持たないホスト(モック・プレビュー)では「読み込めませんでした」。
@MainActor
public protocol FeedHost: AnyObject {
    /// 同じ問い合わせはホストがしばらく覚えておく(画面を行き来するたびに取りに行かない)。`force` で取り直す。
    func feed(_ query: FeedQuery, force: Bool) async throws -> FeedResult
    /// 覚えている行(古くなっていても・取りに行かない)。画面の feed が今出している行と同じ。無ければ nil。
    func cachedFeed(_ query: FeedQuery) -> FeedResult?
}

public extension FeedHost {
    func cachedFeed(_ query: FeedQuery) -> FeedResult? { nil }
}

public extension DSLRuntime {
    var feeds: (any FeedHost)? { self as? any FeedHost }
}
#endif
