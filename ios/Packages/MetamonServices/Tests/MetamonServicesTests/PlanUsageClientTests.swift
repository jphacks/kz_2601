import XCTest
@testable import MetamonServices

/// #317: 使用量は `v1/me/usage`・`v1/apps/{appId}/usage` から読む。古いサーバー(404)は nil で隠す。
final class PlanUsageClientTests: XCTestCase {
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

    private var client: HTTPAppConfigClient { HTTPAppConfigClient(baseURL: base, token: StaticTokenProvider("u1"), cache: nil) }

    func testMyUsagePathAndDecode() async throws {
        StubURLProtocol.reset([.init(status: 200, body: #"""
        {"plan":"free","tier":"signed_in","resetAt":"2026-11-07T15:00:00Z","ai":{"used":12,"limit":60,"remaining":48,"weights":{"create":3,"edit":2,"chat":1,"text":1,"textTask":2}},"apps":{"used":3,"limit":100,"remaining":97},"perApp":{"recordsPerApp":5000,"photosPerApp":500}}
        """#)])
        let me = try await client.myUsage()
        XCTAssertEqual(me?.apps?.used, 3)
        XCTAssertEqual(me?.ai?.remaining, 48)
        let req = try XCTUnwrap(StubURLProtocol.lock.withLock { StubURLProtocol.requests }.first)
        XCTAssertEqual(req.httpMethod, "GET")
        XCTAssertEqual(req.url?.path, "/v1/me/usage")
    }

    func testAppUsagePath() async throws {
        StubURLProtocol.reset([.init(status: 200, body: #"""
        {"appId":"kakeibo-1","plan":"pro","payer":"self","upgrade":false,"records":{"used":120,"limit":50000,"remaining":49880},"photos":{"used":8,"limit":5000,"remaining":4992}}
        """#)])
        let usage = try await client.appUsage(appId: "kakeibo-1")
        XCTAssertEqual(usage?.photos?.limit, 5000)
        XCTAssertEqual(usage?.isMine, true)
        let req = try XCTUnwrap(StubURLProtocol.lock.withLock { StubURLProtocol.requests }.first)
        XCTAssertEqual(req.url?.path, "/v1/apps/kakeibo-1/usage")
    }

    func testOlderServer404IsNil() async throws {
        StubURLProtocol.reset([.init(status: 404, body: #"{"error":{"code":"not_found","message":"not found"}}"#),
                               .init(status: 404, body: #"{"error":{"code":"not_found","message":"not found"}}"#)])
        let me = try await client.myUsage()
        let app = try await client.appUsage(appId: "x")
        XCTAssertNil(me)
        XCTAssertNil(app)
    }
}
