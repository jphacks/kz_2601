import XCTest
import MetamonDSL
import MetamonRenderer
import MetamonServices
@testable import Metamon

/// Mac の feed(#1288): MacAppRuntime を FeedHost にし、iOS と同じ 10 分のキャッシュ(`FeedCache`)を使う
@MainActor
final class MacFeedsTests: XCTestCase {
    private final class CountingFeeds: FeedClient, @unchecked Sendable {
        var calls = 0
        var fail = false
        func feed(_ query: FeedQuery) async throws -> FeedResult {
            calls += 1
            if fail { throw URLError(.notConnectedToInternet) }
            return FeedResult(provider: query.provider, title: "天気 \(calls)", items: [],
                              source: FeedSource(name: "気象庁", url: "https://www.jma.go.jp", license: "CC BY 4.0"))
        }
    }

    private func runtime() throws -> MacAppRuntime {
        let url = try XCTUnwrap(Bundle(for: MacAppRuntime.self).url(forResource: "kakeibo", withExtension: "json"))
        return MacAppRuntime(appId: "test-feeds-\(UUID().uuidString.lowercased())", dsl: try AppDSL.load(contentsOf: url),
                             ai: MockAIClient(delay: .zero), isAIConsentGranted: { false })
    }

    func testRuntimeIsFeedHost() throws {
        let rt = try runtime()
        XCTAssertNotNil((rt as any DSLRuntime).feeds, "部品が FeedHost として見つける")
    }

    func testWithoutClientFails() async throws {
        let rt = try runtime()
        do {
            _ = try await rt.feed(FeedQuery(provider: "weather", region: "none-\(UUID().uuidString)"), force: false)
            XCTFail("サーバーにつながっていない時は読み込めない")
        } catch {}
    }

    func testCachesForTenMinutesAndKeepsStaleOnFailure() async throws {
        let rt = try runtime()
        let client = CountingFeeds()
        rt.feedClient = client
        let query = FeedQuery(provider: "weather", region: "test-\(UUID().uuidString)")

        let first = try await rt.feed(query, force: false)
        let second = try await rt.feed(query, force: false)
        XCTAssertEqual(client.calls, 1, "10 分の間は取りに行かない")
        XCTAssertEqual(first, second)

        client.fail = true
        do {
            _ = try await rt.feed(query, force: true)
            XCTFail("取り直しに失敗したら投げる")
        } catch {}
        XCTAssertEqual(rt.cachedFeed(query), first, "取れなかった時は前の内容を出せる")
    }

    func testMockServicesHaveNoFeeds() {
        XCTAssertNil(MacServices.make(environment: ["METAMON_MOCK": "1"]).feeds)
    }
}
