import XCTest
import MetamonDSL
@testable import MetamonServices

/// 応答を順に返す URLProtocol スタブ(`HTTP.session` に差し込む)。受けたリクエストも記録する。
final class StubURLProtocol: URLProtocol {
    struct Canned {
        var status: Int
        var body: String = ""
        var headers: [String: String] = [:]
    }
    nonisolated(unsafe) static var queue: [Canned] = []
    nonisolated(unsafe) static var requests: [URLRequest] = []
    static let lock = NSLock()

    static func reset(_ responses: [Canned]) {
        lock.withLock {
            queue = responses
            requests = []
        }
    }

    static var authorizations: [String?] { lock.withLock { requests.map { $0.value(forHTTPHeaderField: "Authorization") } } }

    override class func canInit(with request: URLRequest) -> Bool { true }
    override class func canonicalRequest(for request: URLRequest) -> URLRequest { request }

    override func startLoading() {
        let canned: Canned = Self.lock.withLock {
            Self.requests.append(request)
            return Self.queue.isEmpty ? Canned(status: 599, body: "no canned response") : Self.queue.removeFirst()
        }
        let response = HTTPURLResponse(url: request.url!, statusCode: canned.status, httpVersion: "HTTP/1.1", headerFields: canned.headers)!
        client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed)
        client?.urlProtocol(self, didLoad: Data(canned.body.utf8))
        client?.urlProtocolDidFinishLoading(self)
    }

    override func stopLoading() {}
}

/// `idToken()` は固定、`forceRefresh: true` のたびに新しい値を返す TokenProvider(呼び出し回数も記録)。
final class RefreshingToken: TokenProvider, @unchecked Sendable {
    private let lock = NSLock()
    private(set) var refreshCount = 0
    private var current: String

    init(_ initial: String = "tok-1") { current = initial }

    func idToken() async throws -> String? { lock.withLock { current } }

    func idToken(forceRefresh: Bool) async throws -> String? {
        lock.withLock {
            if forceRefresh {
                refreshCount += 1
                current = "tok-\(refreshCount + 1)"
            }
            return current
        }
    }
}

final class ErrorRecoveryTests: XCTestCase {
    private let base = URL(string: "https://ai.example.test/")!
    private var savedSession: URLSession!
    private var savedSleep: (@Sendable (TimeInterval) async -> Void)!
    private final class SleepLog: @unchecked Sendable {
        private let lock = NSLock()
        private(set) var waits: [TimeInterval] = []
        func add(_ s: TimeInterval) { lock.withLock { waits.append(s) } }
    }
    private var sleeps: SleepLog!

    override func setUp() {
        super.setUp()
        savedSession = HTTP.session
        savedSleep = HTTP.sleep
        sleeps = SleepLog()
        HTTP.session = HTTP.makeSession(protocolClasses: [StubURLProtocol.self])
        let log = sleeps!
        HTTP.sleep = { log.add($0) }
    }

    override func tearDown() {
        HTTP.session = savedSession
        HTTP.sleep = savedSleep
        super.tearDown()
    }

    private struct Body: Encodable { var transcript: String }

    // MARK: - 401 → トークン再取得 → 1 回再送

    func test401RefreshesTokenAndRetriesOnce() async throws {
        StubURLProtocol.reset([
            .init(status: 401, body: #"{"error":{"code":"unauthorized","message":"token expired"}}"#),
            .init(status: 200, body: #"{"ok":true}"#),
        ])
        let token = RefreshingToken("tok-1")
        let data = try await HTTP.send("POST", baseURL: base, path: "v1/generate", body: Body(transcript: "家計簿"), token: token)
        XCTAssertEqual(String(data: data, encoding: .utf8), #"{"ok":true}"#)
        XCTAssertEqual(token.refreshCount, 1)
        XCTAssertEqual(StubURLProtocol.authorizations, ["Bearer tok-1", "Bearer tok-2"], "同じリクエストを新トークンで 1 回だけ再送")
        XCTAssertEqual(StubURLProtocol.requests.map(\.httpMethod), ["POST", "POST"])
    }

    func test401TwiceGivesUpAsSessionExpired() async throws {
        StubURLProtocol.reset([
            .init(status: 401, body: #"{"error":{"code":"unauthorized","message":"expired"}}"#),
            .init(status: 401, body: #"{"error":{"code":"unauthorized","message":"expired"}}"#),
            .init(status: 200, body: "{}"), // 使われない(3 回目は送らない)
        ])
        let token = RefreshingToken()
        do {
            _ = try await HTTP.send("GET", baseURL: base, path: "v1/apps", body: nil, token: token)
            XCTFail("expected 401")
        } catch {
            XCTAssertEqual((error as? APIError)?.status, 401)
            XCTAssertEqual(FailureKind.classify(error), .sessionExpired, "呼び出し側はサインイン画面へ")
        }
        XCTAssertEqual(token.refreshCount, 1, "再取得は 1 回だけ")
        XCTAssertEqual(StubURLProtocol.requests.count, 2, "再送は 1 回だけ")
    }

    func test401WithoutTokenDoesNotRetry() async throws {
        StubURLProtocol.reset([.init(status: 401, body: "{}"), .init(status: 200, body: "{}")])
        do {
            _ = try await HTTP.send("GET", baseURL: base, path: "v1/apps", body: nil, token: NoAuthTokenProvider())
            XCTFail("expected 401")
        } catch {
            XCTAssertEqual((error as? APIError)?.status, 401)
        }
        XCTAssertEqual(StubURLProtocol.requests.count, 1, "トークンが無ければ再取得のしようがない")
    }

    func test401WithStaticTokenDoesNotRetry() async throws {
        StubURLProtocol.reset([.init(status: 401, body: "{}"), .init(status: 200, body: "{}")])
        _ = try? await HTTP.send("GET", baseURL: base, path: "v1/apps", body: nil, token: StaticTokenProvider("dev-uid"))
        XCTAssertEqual(StubURLProtocol.requests.count, 1, "forceRefresh しても同じ値なら再送しない")
    }

    // MARK: - 429 Retry-After

    func test429WaitsRetryAfterThenRetriesOnce() async throws {
        StubURLProtocol.reset([
            .init(status: 429, body: #"{"error":{"code":"rate_limited","message":"Claude rate limit"}}"#, headers: ["Retry-After": "3"]),
            .init(status: 200, body: #"{"ok":true}"#),
        ])
        let data = try await HTTP.send("POST", baseURL: base, path: "v1/generate", body: Body(transcript: "x"), token: StaticTokenProvider("u"))
        XCTAssertEqual(String(data: data, encoding: .utf8), #"{"ok":true}"#)
        XCTAssertEqual(sleeps.waits, [3])
        XCTAssertEqual(StubURLProtocol.requests.count, 2)
    }

    func test429TwiceSurfacesBusyWithRetryAfter() async throws {
        StubURLProtocol.reset([
            .init(status: 429, body: "", headers: ["Retry-After": "1"]),
            .init(status: 429, body: #"{"error":{"code":"rate_limited","message":"still busy"}}"#, headers: ["Retry-After": "30"]),
            .init(status: 200, body: "{}"),
        ])
        do {
            _ = try await HTTP.send("POST", baseURL: base, path: "v1/generate", body: Body(transcript: "x"), token: StaticTokenProvider("u"))
            XCTFail("expected 429")
        } catch {
            let api = try XCTUnwrap(error as? APIError)
            XCTAssertEqual(api.status, 429)
            XCTAssertEqual(api.retryAfter, 30)
            XCTAssertEqual(FailureKind.classify(error), .busy(retryAfter: 30))
        }
        XCTAssertEqual(sleeps.waits, [1], "自動再送は 1 回だけ")
        XCTAssertEqual(StubURLProtocol.requests.count, 2)
    }

    func test429WithoutHeaderUsesDefaultWaitAndLongWaitIsNotAutoRetried() async throws {
        StubURLProtocol.reset([.init(status: 429, body: ""), .init(status: 200, body: "{}")])
        _ = try await HTTP.send("GET", baseURL: base, path: "v1/apps", body: nil, token: NoAuthTokenProvider())
        XCTAssertEqual(sleeps.waits, [HTTP.defaultRetryAfter])

        StubURLProtocol.reset([.init(status: 429, body: "", headers: ["Retry-After": "120"]), .init(status: 200, body: "{}")])
        _ = try? await HTTP.send("GET", baseURL: base, path: "v1/apps", body: nil, token: NoAuthTokenProvider())
        XCTAssertEqual(sleeps.waits, [HTTP.defaultRetryAfter], "上限(15s)超の Retry-After は待たずに UI へ")
        XCTAssertEqual(StubURLProtocol.requests.count, 1)
    }

    func testRetryAfterParsesSecondsAndHTTPDate() {
        XCTAssertEqual(HTTP.retryAfter(.init(status: 429, data: Data(), headers: ["Retry-After": "7"])), 7)
        XCTAssertEqual(HTTP.retryAfter(.init(status: 429, data: Data(), headers: ["retry-after": " 2 "])), 2)
        XCTAssertNil(HTTP.retryAfter(.init(status: 429, data: Data(), headers: [:])))
        let past = HTTP.retryAfter(.init(status: 429, data: Data(), headers: ["Retry-After": "Wed, 21 Oct 2015 07:28:00 GMT"]))
        XCTAssertEqual(past, 0, "過去の日時は 0 秒")
    }

    // MARK: - 分類(状態カードはこれだけを見る)

    func testFailureKindClassification() {
        XCTAssertEqual(FailureKind.classify(URLError(.notConnectedToInternet)), .offline)
        XCTAssertEqual(FailureKind.classify(URLError(.networkConnectionLost)), .offline)
        XCTAssertEqual(FailureKind.classify(URLError(.cannotConnectToHost)), .offline)
        XCTAssertEqual(FailureKind.classify(URLError(.timedOut)), .timeout)
        XCTAssertEqual(FailureKind.classify(URLError(.cancelled)), .cancelled)
        XCTAssertEqual(FailureKind.classify(CancellationError()), .cancelled)
        XCTAssertEqual(FailureKind.classify(APIError.server(status: 401, code: "unauthorized", message: "")), .sessionExpired)
        XCTAssertEqual(FailureKind.classify(APIError.server(status: 409, code: "version_conflict", message: "")), .conflict)
        XCTAssertEqual(FailureKind.classify(APIError.server(status: 500, code: "internal", message: "boom")), .busy(retryAfter: nil))
        XCTAssertEqual(FailureKind.classify(APIError.server(status: 503, code: "upstream_unavailable", message: "")), .busy(retryAfter: nil))
        XCTAssertEqual(FailureKind.classify(APIError.http(status: 502, body: "bad gateway")), .busy(retryAfter: nil))
        XCTAssertEqual(FailureKind.classify(APIError.server(status: 504, code: "upstream_timeout", message: "")), .timeout)
        XCTAssertEqual(FailureKind.classify(APIError.server(status: 422, code: "invalid_record", message: "amount")), .rejected("amount"))
        XCTAssertTrue(FailureKind.offline.isRetryable)
        XCTAssertTrue(FailureKind.busy(retryAfter: 2).isRetryable)
        XCTAssertFalse(FailureKind.sessionExpired.isRetryable)
        XCTAssertFalse(FailureKind.conflict.isRetryable)
    }

    // MARK: - 圏外の遅延送信(Connectivity モック)

    func testDeferredSendHoldsWhileOfflineAndSendsOnceOnReconnect() async throws {
        let net = ManualConnectivity(online: false)
        let gate = DeferredSend<String>(connectivity: net)
        let sent = SentLog()
        let held = gate.holdIfOffline("家計簿アプリが欲しい") { sent.add($0) }
        XCTAssertTrue(held)
        XCTAssertEqual(gate.pending, "家計簿アプリが欲しい")
        XCTAssertTrue(sent.items.isEmpty)

        net.set(online: true)
        try await waitUntil { sent.items.count == 1 }
        XCTAssertEqual(sent.items, ["家計簿アプリが欲しい"])
        XCTAssertNil(gate.pending)

        // 再度オフライン→オンラインしても二重送信しない
        net.set(online: false)
        net.set(online: true)
        try await Task.sleep(for: .milliseconds(100))
        XCTAssertEqual(sent.items.count, 1)
    }

    func testDeferredSendIsBypassedWhenOnline() {
        let gate = DeferredSend<String>(connectivity: ManualConnectivity(online: true))
        let sent = SentLog()
        XCTAssertFalse(gate.holdIfOffline("x") { sent.add($0) }, "オンラインなら呼び出し側が即送る")
        XCTAssertNil(gate.pending)
    }

    func testDeferredSendReplacesAndCancels() async throws {
        let net = ManualConnectivity(online: false)
        let gate = DeferredSend<String>(connectivity: net)
        let sent = SentLog()
        gate.holdIfOffline("古い発話") { sent.add($0) }
        gate.holdIfOffline("新しい発話") { sent.add($0) }
        XCTAssertEqual(gate.pending, "新しい発話", "溜めずに置き換える")
        net.set(online: true)
        try await waitUntil { sent.items.count == 1 }
        XCTAssertEqual(sent.items, ["新しい発話"])

        net.set(online: false)
        gate.holdIfOffline("やめる発話") { sent.add($0) }
        gate.cancel()
        XCTAssertNil(gate.pending)
        net.set(online: true)
        try await Task.sleep(for: .milliseconds(100))
        XCTAssertEqual(sent.items, ["新しい発話"], "取り消した保留は復帰しても送らない")
    }

    private final class SentLog: @unchecked Sendable {
        private let lock = NSLock()
        private(set) var items: [String] = []
        func add(_ s: String) { lock.withLock { items.append(s) } }
    }

    private func waitUntil(timeout: TimeInterval = 2, _ cond: @escaping () -> Bool) async throws {
        let deadline = Date().addingTimeInterval(timeout)
        while !cond() {
            if Date() > deadline { return XCTFail("timed out") }
            try await Task.sleep(for: .milliseconds(10))
        }
    }
}
