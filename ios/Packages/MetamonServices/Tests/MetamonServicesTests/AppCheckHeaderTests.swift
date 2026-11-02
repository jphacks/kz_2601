import XCTest
@testable import MetamonServices

/// `forceRefresh: true` のたびに新しい値を返す App Check のトークン(呼び出しも記録)。
private final class FakeAppCheck: AppCheckTokenProvider, @unchecked Sendable {
    private let lock = NSLock()
    private var current: String?
    private(set) var refreshCount = 0

    init(_ initial: String?) { current = initial }

    func token(forceRefresh: Bool) async -> String? {
        lock.withLock {
            if forceRefresh {
                refreshCount += 1
                current = "ac-\(refreshCount + 1)"
            }
            return current
        }
    }
}

/// App Check(#318): ai / appconfig へのリクエストに `X-Firebase-AppCheck` を付け、401 `app_check_failed` を ID トークン失効と区別する。
final class AppCheckHeaderTests: XCTestCase {
    private let base = URL(string: "https://appconfig.example.test/")!
    private let rejected = #"{"error":{"code":"app_check_failed","message":"app check token is missing or invalid","details":{"reason":"invalid"}}}"#
    private var savedSession: URLSession!

    override func setUp() {
        super.setUp()
        savedSession = HTTP.session
        HTTP.session = HTTP.makeSession(protocolClasses: [StubURLProtocol.self])
    }

    override func tearDown() {
        HTTP.session = savedSession
        HTTP.appCheck = nil
        super.tearDown()
    }

    private var appCheckHeaders: [String?] {
        StubURLProtocol.lock.withLock { StubURLProtocol.requests.map { $0.value(forHTTPHeaderField: "X-Firebase-AppCheck") } }
    }

    func testAttachesHeaderWithClientInfo() async throws {
        HTTP.appCheck = FakeAppCheck("ac-1")
        StubURLProtocol.reset([.init(status: 200, body: "{}")])
        _ = try await HTTP.send("GET", baseURL: base, path: "v1/apps", body: nil, token: RefreshingToken())
        XCTAssertEqual(appCheckHeaders, ["ac-1"])
        let req = try XCTUnwrap(StubURLProtocol.requests.first)
        XCTAssertEqual(req.value(forHTTPHeaderField: "Authorization"), "Bearer tok-1")
        XCTAssertEqual(req.value(forHTTPHeaderField: "X-Client-Platform"), "ios")
    }

    func testNoProviderOrNoTokenSendsWithoutHeader() async throws {
        StubURLProtocol.reset([.init(status: 200, body: "{}"), .init(status: 200, body: "{}")])
        _ = try await HTTP.send("GET", baseURL: base, path: "v1/apps", body: nil, token: RefreshingToken())
        HTTP.appCheck = FakeAppCheck(nil) // 取得失敗(デバッグ用トークン未登録など)
        _ = try await HTTP.send("GET", baseURL: base, path: "v1/apps", body: nil, token: RefreshingToken())
        XCTAssertEqual(appCheckHeaders, [nil, nil], "ヘッダ無しでも送る(サーバーが off / log なら通る)")
    }

    func testAppCheckRejectionRefreshesAppCheckNotIDToken() async throws {
        let appCheck = FakeAppCheck("ac-1")
        HTTP.appCheck = appCheck
        StubURLProtocol.reset([.init(status: 401, body: rejected), .init(status: 200, body: #"{"ok":true}"#)])
        let token = RefreshingToken()
        let data = try await HTTP.send("POST", baseURL: base, path: "v1/generate", body: ["t": "x"], token: token)
        XCTAssertEqual(String(data: data, encoding: .utf8), #"{"ok":true}"#)
        XCTAssertEqual(appCheckHeaders, ["ac-1", "ac-2"], "App Check のトークンを取り直して 1 回だけ再送")
        XCTAssertEqual(appCheck.refreshCount, 1)
        XCTAssertEqual(token.refreshCount, 0, "ID トークンは取り直さない")
    }

    func testPersistentAppCheckRejectionIsNotSessionExpired() async throws {
        HTTP.appCheck = FakeAppCheck("ac-1")
        StubURLProtocol.reset([.init(status: 401, body: rejected), .init(status: 401, body: rejected), .init(status: 200, body: "{}")])
        do {
            _ = try await HTTP.send("GET", baseURL: base, path: "v1/apps", body: nil, token: RefreshingToken())
            XCTFail("expected 401")
        } catch {
            let api = try XCTUnwrap(error as? APIError)
            XCTAssertTrue(api.isAppCheckFailure)
            XCTAssertEqual(FailureKind.classify(error), .appCheckFailed, "サインイン画面へは戻さない")
        }
        XCTAssertEqual(StubURLProtocol.requests.count, 2, "再送は 1 回だけ")
    }

    func testIDTokenExpiryStillRefreshesWithAppCheckAttached() async throws {
        HTTP.appCheck = FakeAppCheck("ac-1")
        StubURLProtocol.reset([
            .init(status: 401, body: #"{"error":{"code":"unauthenticated","message":"invalid token"}}"#),
            .init(status: 200, body: "{}"),
        ])
        _ = try await HTTP.send("GET", baseURL: base, path: "v1/apps", body: nil, token: RefreshingToken())
        XCTAssertEqual(StubURLProtocol.authorizations, ["Bearer tok-1", "Bearer tok-2"])
        XCTAssertEqual(appCheckHeaders, ["ac-1", "ac-1"])
    }

    func testFirstValueTimesOut() async {
        let started = Date()
        let value = await firstValue(timeout: 0.05) { _ in } // completion が来ない
        XCTAssertNil(value)
        XCTAssertLessThan(Date().timeIntervalSince(started), 2)
        let fast = await firstValue(timeout: 5) { done in done("tok") }
        XCTAssertEqual(fast, "tok")
    }
}
