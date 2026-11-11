import XCTest
@testable import MetamonServices

/// #1493: 外の API の口(openapi-appconfig.yaml の tag `ext`)の送り方と受け方。
final class ExternalServicesClientTests: XCTestCase {
    private var savedSession: URLSession!
    private let base = URL(string: "https://appconfig.test/")!
    private var client: HTTPExternalServicesClient { HTTPExternalServicesClient(baseURL: base, token: StaticTokenProvider("u1")) }

    override func setUp() {
        super.setUp()
        savedSession = HTTP.session
        HTTP.session = HTTP.makeSession(protocolClasses: [StubURLProtocol.self])
    }

    override func tearDown() {
        HTTP.session = savedSession
        super.tearDown()
    }

    private var lastRequest: URLRequest? { StubURLProtocol.lock.withLock { StubURLProtocol.requests.last } }

    func testListConnections() async throws {
        StubURLProtocol.reset([.init(status: 200, body: #"""
        {"connections":[
          {"provider":"googleCalendar","status":"active","scopes":["calendar.readonly"],"connectedAt":"2026-11-10T01:00:00Z","lastUsedAt":"2026-11-11T02:00:00.123Z"},
          {"provider":"todoist","status":"error"}
        ]}
        """#)])
        let list = try await client.connections()
        XCTAssertEqual(lastRequest?.httpMethod, "GET")
        XCTAssertEqual(lastRequest?.url?.path, "/v1/me/connections")
        XCTAssertEqual(list.map(\.provider), ["googleCalendar", "todoist"])
        XCTAssertEqual(list[0].status, .active)
        XCTAssertEqual(list[0].scopes, ["calendar.readonly"])
        XCTAssertNotNil(list[0].connectedAt)
        XCTAssertNotNil(list[0].lastUsedAt, "小数秒つきも読む")
        XCTAssertEqual(list[1].status, .error)
        XCTAssertNil(list[1].connectedAt)
    }

    func testUnknownStatusDecodes() throws {
        let c = try JSONDecoder().decode(ExtConnection.self, from: Data(#"{"provider":"x1","status":"paused"}"#.utf8))
        XCTAssertEqual(c.status, .unknown("paused"))
    }

    func testStartConnectionWithoutScopesSendsEmptyBody() async throws {
        StubURLProtocol.reset([.init(status: 200, body: #"{"authUrl":"https://accounts.example.com/o/oauth2/auth?state=s1"}"#)])
        let url = try await client.startConnection(provider: "googleCalendar", scopes: nil)
        XCTAssertEqual(url.host, "accounts.example.com")
        let req = try XCTUnwrap(lastRequest)
        XCTAssertEqual(req.httpMethod, "POST")
        XCTAssertEqual(req.url?.path, "/v1/me/connections/googleCalendar/start")
        XCTAssertEqual(StubURLProtocol.bodyJSON(req), [:], "scope を書かなければ読む op の分だけ(本文は空のオブジェクト)")
    }

    func testStartConnectionWithScopes() async throws {
        StubURLProtocol.reset([.init(status: 200, body: #"{"authUrl":"https://todoist.example.com/oauth?state=s"}"#)])
        _ = try await client.startConnection(provider: "todoist", scopes: ["data:read_write"])
        let body = try XCTUnwrap(StubURLProtocol.bodyJSON(try XCTUnwrap(lastRequest)))
        XCTAssertEqual(body["scopes"] as? [String], ["data:read_write"])
    }

    func testStartConnectionRejectsNonHTTPSURL() async {
        StubURLProtocol.reset([.init(status: 200, body: #"{"authUrl":"javascript:alert(1)"}"#)])
        do {
            _ = try await client.startConnection(provider: "todoist", scopes: nil)
            XCTFail("https 以外は開かない")
        } catch {}
    }

    func testDisconnect() async throws {
        StubURLProtocol.reset([.init(status: 204)])
        try await client.disconnect(provider: "todoist")
        XCTAssertEqual(lastRequest?.httpMethod, "DELETE")
        XCTAssertEqual(lastRequest?.url?.path, "/v1/me/connections/todoist")
    }

    func testProviderMustBeAName() async {
        StubURLProtocol.reset([])
        for bad in ["../admin", "a", "Google", "x/y", "a?b=1", ""] {
            do {
                try await client.disconnect(provider: bad)
                XCTFail("\(bad) は送らない")
            } catch {}
        }
        XCTAssertTrue(StubURLProtocol.lock.withLock { StubURLProtocol.requests.isEmpty })
    }

    func testCallShapeAndCoinHeaders() async throws {
        StubURLProtocol.reset([.init(status: 200, body: #"{"rows":[{"title":"USD","value":"0.0066","subtitle":"2026-11-11・1 JPY 基準"}]}"#,
                                     headers: ["X-Coins-Charged": "3", "X-Coins-Balance": "97"])])
        let r = try await client.call(appId: "app_01", provider: "frankfurter", op: "latest", args: ["from": "JPY", "to": "USD"], requestId: "req-1")
        let req = try XCTUnwrap(lastRequest)
        XCTAssertEqual(req.httpMethod, "POST")
        XCTAssertEqual(req.url?.path, "/v1/apps/app_01/ext/frankfurter/latest")
        XCTAssertEqual(req.value(forHTTPHeaderField: "X-Request-Id"), "req-1")
        let body = try XCTUnwrap(StubURLProtocol.bodyJSON(req))
        XCTAssertEqual(body["args"] as? [String: String], ["from": "JPY", "to": "USD"])
        XCTAssertEqual(Set(body.allKeys as? [String] ?? []), ["args"])
        XCTAssertEqual(r.result.rows, [ExtRow(title: "USD", subtitle: "2026-11-11・1 JPY 基準", value: "0.0066")])
        XCTAssertEqual(r.coinsCharged, 3)
        XCTAssertEqual(r.coinsBalance, 97)
    }

    func testCallWithoutCoinHeaders() async throws {
        StubURLProtocol.reset([.init(status: 200, body: #"{"resultId":"evt_1"}"#)])
        let r = try await client.call(appId: "a1", provider: "googleCalendar", op: "createEvent", args: [:], requestId: nil)
        XCTAssertNil(lastRequest?.value(forHTTPHeaderField: "X-Request-Id"))
        XCTAssertEqual(r.result.resultId, "evt_1")
        XCTAssertNil(r.coinsCharged)
    }

    func testErrorCodes() async {
        let cases: [(Int, String, (APIError) -> Bool)] = [
            (409, "ext_not_connected", { $0.isExtNotConnected }),
            (503, "ext_unavailable", { $0.isExtUnavailable }),
            (503, "ext_busy", { $0.isExtBusy }),
        ]
        for (status, code, check) in cases {
            StubURLProtocol.reset([.init(status: status, body: #"{"error":{"code":"\#(code)","message":"m","details":{"provider":"todoist"}}}"#)])
            do {
                _ = try await client.call(appId: "a1", provider: "todoist", op: "listTasks", args: [:], requestId: nil)
                XCTFail("\(code) は失敗")
            } catch let e as APIError {
                XCTAssertTrue(check(e), code)
                XCTAssertEqual(e.extProvider, "todoist")
            } catch {
                XCTFail("\(error)")
            }
        }
    }

    func testMockConnectsAndDisconnects() async throws {
        let mock = MockExternalServicesClient()
        let url = try await mock.startConnection(provider: "todoist", scopes: nil)
        XCTAssertTrue(MockExternalServicesClient.isMockAuthURL(url))
        var list = try await mock.connections()
        XCTAssertEqual(list.map(\.provider), ["todoist"])
        try await mock.disconnect(provider: "todoist")
        list = try await mock.connections()
        XCTAssertTrue(list.isEmpty)
    }
}
