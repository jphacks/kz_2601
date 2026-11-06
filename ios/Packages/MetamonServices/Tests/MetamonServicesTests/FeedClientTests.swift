#if !APP_STORE_REVIEW
import XCTest
import MetamonDSL
@testable import MetamonServices

/// #348: 外部の公開データは appconfig の `v1/feeds/{provider}` だけに聞く(外部へ直接つながらない)。
final class FeedClientTests: XCTestCase {
    private var savedSession: URLSession!
    private let base = URL(string: "https://appconfig.test/")!

    override func setUp() {
        super.setUp()
        savedSession = HTTP.session
        HTTP.session = HTTP.makeSession(protocolClasses: [StubURLProtocol.self])
    }

    override func tearDown() {
        HTTP.session = savedSession
        super.tearDown()
    }

    private let body = #"""
    {"provider":"weather","title":"石川県の天気","fetchedAt":"2026-11-06T03:00:00Z","source":{"name":"気象庁ホームページ","url":"https://www.jma.go.jp/","license":"公共データ利用規約(第1.0版)"},
     "items":[{"date":"2026-11-07","title":"雨","icon":"cloud.rain","max":13,"min":9,"pop":80},{"bad":true},{"date":"2026-11-08","title":"晴れ"}]}
    """#

    func testRequestAndDecode() async throws {
        StubURLProtocol.reset([.init(status: 200, body: body)])
        let client = HTTPFeedClient(baseURL: base, token: StaticTokenProvider("u1"))
        let r = try await client.feed(FeedQuery(provider: "weather", region: "170000", limit: 3))
        XCTAssertEqual(r.title, "石川県の天気")
        XCTAssertEqual(r.items.map(\.title), ["雨", "晴れ"], "壊れた行だけ落とす")
        XCTAssertEqual(r.items.first?.pop, 80)
        XCTAssertEqual(r.source.name, "気象庁ホームページ")
        XCTAssertFalse(r.stale)
        let req = try XCTUnwrap(StubURLProtocol.lock.withLock { StubURLProtocol.requests }.first)
        XCTAssertEqual(req.httpMethod, "GET")
        XCTAssertEqual(req.url?.host(), "appconfig.test")
        XCTAssertEqual(req.url?.path, "/v1/feeds/weather")
        XCTAssertEqual(req.url?.query, "region=170000&limit=3")
    }

    @MainActor
    func testCacheKeepsSameQuery() async throws {
        actor Counter: FeedClient {
            var calls = 0
            func feed(_ query: FeedQuery) async throws -> FeedResult {
                calls += 1
                return FeedResult(provider: query.provider, title: "t", items: [], source: FeedSource(name: "n", url: "https://x", license: "l"))
            }
        }
        var now = Date(timeIntervalSince1970: 0)
        let cache = FeedCache(now: { now })
        let client = Counter()
        let q = FeedQuery(provider: "holidays")
        _ = try await cache.load(q, force: false, client: client)
        _ = try await cache.load(q, force: false, client: client)
        var calls = await client.calls
        XCTAssertEqual(calls, 1)
        _ = try await cache.load(q, force: true, client: client)
        now += FeedCache.ttl + 1
        _ = try await cache.load(q, force: false, client: client)
        calls = await client.calls
        XCTAssertEqual(calls, 3)
    }
}
#endif
