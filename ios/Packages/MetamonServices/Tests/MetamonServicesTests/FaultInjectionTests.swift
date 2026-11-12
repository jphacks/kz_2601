import XCTest
import MetamonDSL
@testable import MetamonServices

/// 応答の作り物(#2083): 一致した時だけ返す・次の 1 回で消える・圏外・決まりが無い時は通す
final class FaultInjectionTests: XCTestCase {
    private var savedSession: URLSession!
    private var savedSleep: (@Sendable (TimeInterval) async -> Void)!
    private let base = URL(string: "https://ai.example.test/")!

    private struct Token: TokenProvider {
        func idToken() async throws -> String? { "tok" }
        func idToken(forceRefresh: Bool) async throws -> String? { "tok" }
    }

    override func setUp() {
        super.setUp()
        savedSession = HTTP.session
        savedSleep = HTTP.sleep
        FaultInjection.installsSession = false
        FaultInjection.removeAll()
        // 作り物 → 一致しなければスタブ(= 本物のサーバーの代わり)
        HTTP.session = HTTP.makeSession(protocolClasses: [FaultURLProtocol.self, StubURLProtocol.self])
        HTTP.sleep = { _ in }
    }

    override func tearDown() {
        FaultInjection.removeAll()
        FaultInjection.installsSession = true
        FaultInjection.now = { Date() }
        HTTP.session = savedSession
        HTTP.sleep = savedSleep
        super.tearDown()
    }

    private func req(_ path: String, _ method: String = "POST") -> URLRequest {
        var r = URLRequest(url: URL(string: path, relativeTo: base)!)
        r.httpMethod = method
        return r
    }

    // MARK: 一致

    func testMatchPathPrefixBySegment() {
        let m = FaultMatch(path: "v1/apps")
        XCTAssertTrue(m.matches(req("v1/apps")))
        XCTAssertTrue(m.matches(req("v1/apps/a1/dsl")))
        XCTAssertFalse(m.matches(req("v1/appsx")))
        XCTAssertFalse(m.matches(req("v1/generate")))
    }

    func testMatchWildcardMethodQuery() {
        let ext = FaultMatch(path: "v1/apps/*/ext", method: "POST")
        XCTAssertTrue(ext.matches(req("v1/apps/a1/ext/fakeOauth/listItems")))
        XCTAssertFalse(ext.matches(req("v1/apps/a1/ext/fakeOauth/listItems", "GET")))
        XCTAssertFalse(ext.matches(req("v1/apps/a1/dsl")))
        let stream = FaultMatch(path: "v1/generate", query: "stream=1")
        XCTAssertTrue(stream.matches(req("v1/generate?stream=1")))
        XCTAssertFalse(stream.matches(req("v1/generate")))
        XCTAssertTrue(FaultMatch.all.matches(req("anything", "GET")))
    }

    // MARK: 回数

    func testOnceIsConsumedOnFirstHit() {
        FaultInjection.add(FaultRule(label: "x", match: FaultMatch(path: "v1/generate"), outcome: .offline))
        XCTAssertFalse(FaultInjection.hasMatch(req("v1/chat")))
        XCTAssertTrue(FaultInjection.hasMatch(req("v1/generate")))
        XCTAssertNotNil(FaultInjection.take(req("v1/generate")))
        XCTAssertNil(FaultInjection.take(req("v1/generate")))
        XCTAssertTrue(FaultInjection.rules.isEmpty)
    }

    func testOnceFollowUpCoversResendsThenExpires() {
        var t = Date(timeIntervalSince1970: 1000)
        FaultInjection.now = { [t] in t }
        FaultInjection.add(FaultRule(label: "x", match: .all, outcome: .offline, times: .once(followUp: 20)))
        XCTAssertNotNil(FaultInjection.take(req("v1/generate?stream=1")))
        t += 5
        FaultInjection.now = { [t] in t }
        XCTAssertNotNil(FaultInjection.take(req("v1/generate")), "同じ操作の送り直しにも効く")
        t += 30
        FaultInjection.now = { [t] in t }
        XCTAssertFalse(FaultInjection.hasMatch(req("v1/generate")))
        XCTAssertTrue(FaultInjection.rules.isEmpty)
    }

    func testAlwaysStays() {
        FaultInjection.add(FaultRule(label: "x", match: .all, outcome: .offline, times: .always))
        for _ in 0..<3 { XCTAssertNotNil(FaultInjection.take(req("v1/generate"))) }
        XCTAssertEqual(FaultInjection.rules.first?.hits, 3)
    }

    // MARK: HTTP を通して

    func testNoRulePassesThrough() async throws {
        StubURLProtocol.reset([.init(status: 200, body: "{\"ok\":true}")])
        let data = try await HTTP.send("POST", baseURL: base, path: "v1/generate", body: nil, token: Token())
        XCTAssertEqual(String(decoding: data, as: UTF8.self), "{\"ok\":true}")
        XCTAssertEqual(StubURLProtocol.lock.withLock { StubURLProtocol.requests.count }, 1)
    }

    func testMatchedRuleAnswersWithoutServerAndOthersPass() async throws {
        StubURLProtocol.reset([.init(status: 200, body: "{}")])
        let outcome = FaultOutcome.error(status: 402, code: "app_limit_reached", message: "上限",
                                         details: ["held": 3, "limit": 3, "canBuySlot": false, "apps": [["id": "a1", "name": "家計簿", "mine": true]]])
        FaultInjection.add(FaultRule(label: "x", match: FaultMatch(path: "v1/generate"), outcome: outcome))
        do {
            _ = try await HTTP.send("POST", baseURL: base, path: "v1/generate", body: nil, token: Token())
            XCTFail("402 のはず")
        } catch {
            let limit = try XCTUnwrap(AppLimit(error))
            XCTAssertEqual(limit.held, 3)
            XCTAssertEqual(limit.apps?.first?.name, "家計簿")
            XCTAssertEqual(FailureKind.classify(error), FailureKind.classify(try XCTUnwrap(outcome.expectedError)))
        }
        XCTAssertEqual(StubURLProtocol.lock.withLock { StubURLProtocol.requests.count }, 0, "サーバーへは行かない")
        _ = try await HTTP.send("GET", baseURL: base, path: "v1/apps", body: nil, token: Token())
        XCTAssertEqual(StubURLProtocol.lock.withLock { StubURLProtocol.requests.count }, 1, "一致しない通信は通す")
    }

    func testOffline() async {
        FaultInjection.add(FaultRule(label: "x", match: .all, outcome: .offline))
        do {
            _ = try await HTTP.send("GET", baseURL: base, path: "v1/apps", body: nil, token: Token())
            XCTFail("圏外のはず")
        } catch {
            XCTAssertEqual(FailureKind.classify(error), .offline)
        }
    }

    func testRateLimitedWithLongRetryAfterIsNotResent() async {
        StubURLProtocol.reset([])
        FaultInjection.add(FaultRule(label: "x", match: .all, outcome: .error(
            status: 429, code: "rate_limited", message: "今日の上限", details: ["scope": "user", "limit": 20, "remaining": 0],
            headers: ["Retry-After": "600"]), times: .always))
        do {
            _ = try await HTTP.send("POST", baseURL: base, path: "v1/generate", body: nil, token: Token())
            XCTFail("429 のはず")
        } catch {
            guard case .rateLimited(let info) = FailureKind.classify(error) else { return XCTFail("\(error)") }
            XCTAssertEqual(info.limit, 20)
        }
        XCTAssertEqual(FaultInjection.rules.first?.hits, 1)
    }

    func testErrorBodyMatchesServerShape() throws {
        let body = FaultOutcome.errorBody(code: "pro_required", message: "m", details: ["features": []])
        let obj = try XCTUnwrap(JSONSerialization.jsonObject(with: body) as? [String: Any])
        let inner = try XCTUnwrap(obj["error"] as? [String: Any])
        XCTAssertEqual(inner["code"] as? String, "pro_required")
        XCTAssertNotNil(inner["details"])
        let bare = try XCTUnwrap(JSONSerialization.jsonObject(with: FaultOutcome.errorBody(code: "internal", message: "m")) as? [String: [String: Any]])
        XCTAssertNil(bare["error"]?["details"], "details が無い時は付けない(サーバーの httpx.Error と同じ)")
    }

    func testSSEErrorEventAndCut() {
        let err = FaultOutcome.events(frames: [FaultOutcome.errorFrame(status: 429, code: "rate_limited", message: "m",
                                                                       details: ["scope": "global"])], cut: false)
        guard case .rateLimited(let info) = FailureKind.classify(err.expectedError!) else { return XCTFail() }
        XCTAssertEqual(info.scope, .global)
        let cut = FaultOutcome.events(frames: [FaultOutcome.frame(event: "app", data: "{\"name\":\"x\"}")], cut: true)
        XCTAssertNil(cut.expectedError, "切れたら通常の呼び方に戻る(エラーにしない)")
    }
}
