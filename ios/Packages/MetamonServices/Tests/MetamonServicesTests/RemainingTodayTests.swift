import XCTest
@testable import MetamonServices

/// #630 M1: 応答の `X-RateLimit-*` から「今日の残り」を読む・日付が変わったら古い扱い。
final class RemainingTodayTests: XCTestCase {
    private let now = ISO8601DateFormatter().date(from: "2026-11-14T03:00:00Z")! // 11/14 12:00 JST
    private let reset = ISO8601DateFormatter().date(from: "2026-11-14T15:00:00Z")! // 11/15 0:00 JST

    private func parse(_ headers: [String: String], status: Int = 200, body: String = "{}") -> RemainingToday? {
        let r = HTTP.Response(status: status, data: Data(body.utf8), headers: headers)
        return RemainingToday.parse(header: r.header, status: status, body: r.data, now: now)
    }

    func testReadsAllHeadersCaseInsensitively() throws {
        // Go はヘッダー名を X-Ratelimit-Remaining に正規化して返す
        let v = try XCTUnwrap(parse(["X-Ratelimit-Remaining": "24", "x-ratelimit-limit": "60", "X-RateLimit-Reset": "\(Int(reset.timeIntervalSince1970))"]))
        XCTAssertEqual(v, RemainingToday(remaining: 24, limit: 60, resetAt: reset, observedAt: now))
    }

    func testMissingRemainingIsNil() {
        XCTAssertNil(parse([:]), "appconfig など、ヘッダーの無い応答では何もしない")
        XCTAssertNil(parse(["X-RateLimit-Limit": "60"]))
    }

    func testBrokenValuesAreIgnored() {
        XCTAssertNil(parse(["X-RateLimit-Remaining": "abc"]))
        XCTAssertNil(parse(["X-RateLimit-Remaining": "-1"]))
        XCTAssertNil(parse(["X-RateLimit-Remaining": "2.5"]))
        XCTAssertNil(parse(["X-RateLimit-Remaining": "61", "X-RateLimit-Limit": "60"]), "上限より多い残りは壊れている")
        let v = parse(["X-RateLimit-Remaining": " 5 ", "X-RateLimit-Limit": "x", "X-RateLimit-Reset": "soon"])
        XCTAssertEqual(v?.remaining, 5)
        XCTAssertNil(v?.limit, "壊れた上限は捨てて残りだけ使う")
        XCTAssertNil(v?.resetAt)
    }

    func testRateLimitedOnlyForUserScope() {
        let user = #"{"error":{"code":"rate_limited","message":"x","details":{"scope":"user","remaining":0}}}"#
        let global = #"{"error":{"code":"rate_limited","message":"x","details":{"scope":"global","remaining":3}}}"#
        XCTAssertEqual(parse(["X-RateLimit-Remaining": "0", "X-RateLimit-Limit": "60"], status: 429, body: user)?.remaining, 0)
        XCTAssertNil(parse(["X-RateLimit-Remaining": "3", "X-RateLimit-Limit": "2000"], status: 429, body: global), "全体の上限の数は本人の残りではない")
        XCTAssertNil(parse(["X-RateLimit-Remaining": "0"], status: 429, body: "busy"))
    }

    func testStaleAfterReset() {
        let v = RemainingToday(remaining: 10, limit: 60, resetAt: reset, observedAt: now)
        XCTAssertFalse(v.isStale(now: reset.addingTimeInterval(-1)))
        XCTAssertTrue(v.isStale(now: reset))
    }

    func testStaleWhenJSTDateChangesWithoutReset() {
        let v = RemainingToday(remaining: 10, observedAt: now)
        XCTAssertFalse(v.isStale(now: ISO8601DateFormatter().date(from: "2026-11-14T14:59:00Z")!), "23:59 JST はまだ同じ日")
        XCTAssertTrue(v.isStale(now: ISO8601DateFormatter().date(from: "2026-11-14T15:00:00Z")!), "0:00 JST で古い")
    }

    @MainActor
    func testStoreKeepsNewestPersistsAndClears() throws {
        let defaults = try XCTUnwrap(UserDefaults(suiteName: "RemainingTodayTests"))
        defaults.removePersistentDomain(forName: "RemainingTodayTests")
        let store = RemainingTodayStore(defaults: defaults)
        store.record(RemainingToday(remaining: 20, limit: 60, resetAt: reset, observedAt: now))
        store.record(RemainingToday(remaining: 30, limit: 60, resetAt: reset, observedAt: now.addingTimeInterval(-5)))
        XCTAssertEqual(store.last?.remaining, 20, "後から届いた前の依頼の分では上書きしない")
        XCTAssertEqual(RemainingTodayStore(defaults: defaults).last?.remaining, 20, "起動し直しても最後の値が残る")
        XCTAssertNotNil(store.current(now: now))
        XCTAssertNil(store.current(now: reset), "戻る時刻を過ぎたら出さない")
        store.clear()
        XCTAssertNil(store.last)
        XCTAssertNil(RemainingTodayStore(defaults: defaults).last)
    }

    func testHTTPResponsesFeedTheSink() async throws {
        final class Box: @unchecked Sendable {
            let lock = NSLock()
            var values: [RemainingToday] = []
        }
        let box = Box()
        let savedSession = HTTP.session, savedSink = HTTP.remainingSink
        defer { HTTP.session = savedSession; HTTP.remainingSink = savedSink }
        HTTP.session = HTTP.makeSession(protocolClasses: [StubURLProtocol.self])
        HTTP.remainingSink = { v in box.lock.withLock { box.values.append(v) } }
        StubURLProtocol.reset([
            .init(status: 200, body: "{}", headers: ["X-RateLimit-Remaining": "57", "X-RateLimit-Limit": "60"]),
            .init(status: 200, body: "{}"),
        ])
        _ = try await HTTP.send("POST", baseURL: URL(string: "https://ai.example.test/")!, path: "v1/chat", body: nil, token: RefreshingToken())
        _ = try await HTTP.send("GET", baseURL: URL(string: "https://appconfig.example.test/")!, path: "v1/apps", body: nil, token: RefreshingToken())
        XCTAssertEqual(box.lock.withLock { box.values.map(\.remaining) }, [57], "ヘッダーの無い応答は数えない")
    }
}
