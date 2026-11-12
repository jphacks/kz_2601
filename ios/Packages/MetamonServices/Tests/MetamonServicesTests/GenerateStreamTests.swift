import XCTest
import MetamonDSL
@testable import MetamonServices

/// 本文を断片に分けて返し、最後に切断(エラー)もできる URLProtocol スタブ(偽の SSE・#636)。
final class ChunkedStubURLProtocol: URLProtocol {
    struct Canned {
        var status: Int = 200
        var contentType = "text/event-stream"
        var chunks: [String] = []
        /// 送り終えた後に切る
        var failAfter: URLError.Code?
    }
    nonisolated(unsafe) static var queue: [Canned] = []
    nonisolated(unsafe) static var urls: [URL] = []
    static let lock = NSLock()

    static func reset(_ responses: [Canned]) {
        lock.withLock {
            queue = responses
            urls = []
        }
    }

    static var paths: [String] {
        lock.withLock { urls.map { u in u.path + (u.query.map { "?" + $0 } ?? "") } }
    }

    override class func canInit(with request: URLRequest) -> Bool { true }
    override class func canonicalRequest(for request: URLRequest) -> URLRequest { request }

    override func startLoading() {
        let canned: Canned = Self.lock.withLock {
            Self.urls.append(request.url!)
            return Self.queue.isEmpty ? Canned(status: 599, contentType: "text/plain", chunks: ["no canned response"]) : Self.queue.removeFirst()
        }
        let response = HTTPURLResponse(url: request.url!, statusCode: canned.status, httpVersion: "HTTP/1.1",
                                       headerFields: ["Content-Type": canned.contentType])!
        client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed)
        for chunk in canned.chunks { client?.urlProtocol(self, didLoad: Data(chunk.utf8)) }
        if let code = canned.failAfter {
            client?.urlProtocol(self, didFailWithError: URLError(code))
        } else {
            client?.urlProtocolDidFinishLoading(self)
        }
    }

    override func stopLoading() {}
}

private final class ProgressLog: @unchecked Sendable {
    private let lock = NSLock()
    private var items: [GenerateProgress] = []
    func add(_ p: GenerateProgress) { lock.withLock { items.append(p) } }
    var all: [GenerateProgress] { lock.withLock { items } }
}

private struct FixedToken: TokenProvider {
    func idToken() async throws -> String? { "tok" }
    func idToken(forceRefresh: Bool) async throws -> String? { "tok" }
}

/// 途中経過を持たない AIClient(既定の実装を通る)。
private struct PlainAI: AIClient {
    func morph(transcript: String, appId: String, currentVersion: Int) async throws -> MorphResult {
        try HTTPAIClient.parse(Data(GenerateStreamTests.resultJSON.utf8), appId: appId)
    }
    func chat(appId: String, messages: [ChatMessage], locale: String?, mode: ChatMode?) async throws -> ChatTurn { throw URLError(.timedOut) }
    func resetChat(appId: String) async {}
}

final class GenerateStreamTests: XCTestCase {
    private let base = URL(string: "https://ai.example.test/")!
    private var savedSession: URLSession!

    override func setUp() {
        super.setUp()
        savedSession = HTTP.session
        HTTP.session = HTTP.makeSession(protocolClasses: [ChunkedStubURLProtocol.self])
    }

    override func tearDown() {
        HTTP.session = savedSession
        super.tearDown()
    }

    private var client: HTTPAIClient { HTTPAIClient(baseURL: base, token: FixedToken(), cache: nil) }

    static let resultJSON = #"""
    {"appId":"a1","version":3,"mode":"full","summary":"できました","dsl":{"schemaVersion":1,"app":{"name":"家計簿","voiceEntry":"none"},"initialScreen":"home","entities":[],"screens":[{"id":"home","title":"家計簿","components":[{"id":"t","type":"text","text":"家計簿","area":"A1:F1"}]}]}}
    """#

    private static func frame(_ event: String, _ data: String) -> String { "event: \(event)\ndata: \(data)\n\n" }

    // MARK: - 行の組み立て

    func testParserJoinsSplitChunksAndSkipsKeepAlive() {
        var p = SSEParser()
        var frames: [SSEFrame] = []
        for piece in [": keep-alive\n\nev", "ent: app\r\nda", "ta: {\"name\":\"x\"}\r", "\n\r\n", "id: 7\nretry: 10\n\n"] {
            frames += p.feed(Array(piece.utf8))
        }
        XCTAssertEqual(frames, [SSEFrame(event: "app", data: #"{"name":"x"}"#)], "keep-alive・id・retry は 1 件にしない")
    }

    func testParserJoinsMultipleDataLinesAndDropsUnterminatedFrame() {
        var p = SSEParser()
        let frames = p.feed(Array("data: a\ndata:b\n\nevent: result\ndata: {\"cut".utf8))
        XCTAssertEqual(frames, [SSEFrame(event: "message", data: "a\nb")], "区切りの空行が来ない最後の 1 件は出さない")
    }

    func testDecodeTypesEventsAndSkipsBrokenOnes() throws {
        func d(_ e: String, _ data: String) -> GenerateStreamEvent? { GenerateStreamEvent.decode(SSEFrame(event: e, data: data)) }
        guard case .progress(.app(let name, let tabs, let attempt))? = d("app", #"{"type":"app","attempt":1,"locale":"ja","name":" 家計簿 ","tabs":["ホーム","集計"]}"#) else {
            return XCTFail("app")
        }
        XCTAssertEqual(name, "家計簿")
        XCTAssertEqual(tabs, ["ホーム", "集計"])
        XCTAssertEqual(attempt, 1)

        guard case .progress(.component(let c))? = d("component",
            #"{"type":"component","attempt":2,"screen":"home","component":{"index":1,"id":"total","type":"stat","area":"A3:C5","label":"合計"}}"#) else {
            return XCTFail("component")
        }
        XCTAssertEqual(c, StreamedComponent(attempt: 2, screen: "home", index: 1, id: "total", type: "stat", area: "A3:C5", label: "合計"))

        guard case .progress(.restart(2))? = d("restart", #"{"type":"restart","attempt":2}"#) else { return XCTFail("restart") }
        guard case .failure(let e)? = d("error", #"{"error":{"code":"upstream_timeout","message":"遅い","status":504}}"#) else {
            return XCTFail("error")
        }
        XCTAssertEqual(e.status, 504)
        XCTAssertEqual(e.code, "upstream_timeout")

        XCTAssertNil(d("component", "{壊れた"), "壊れた行は読み飛ばす")
        XCTAssertNil(d("component", #"{"component":{"index":0,"type":""}}"#), "type の無い部品は出さない")
        XCTAssertNil(d("app", #"{"name":"  "}"#))
        XCTAssertNil(d("summary", #"{"summary":"作っています"}"#), "iOS が使わない event")
    }

    // MARK: - 生成

    func testStreamDeliversProgressInOrderThenResult() async throws {
        ChunkedStubURLProtocol.reset([.init(chunks: [
            ": ping\n\n",
            Self.frame("app", #"{"type":"app","attempt":1,"name":"家計簿","tabs":["ホーム","集計"]}"#),
            Self.frame("component", #"{"attempt":1,"screen":"home","component":{"index":0,"id":"t","type":"text","area":"A1:F1","label":"家計簿"}}"#),
            "event: component\ndata: {壊れた}\n\n",
            Self.frame("restart", #"{"attempt":2}"#),
            Self.frame("component", #"{"attempt":2,"screen":"home","component":{"index":0,"id":"t","type":"text","area":"A1:F1"}}"#),
            "event: result\ndata: " + Self.resultJSON.prefix(40), // 断片の途中で区切る
            String(Self.resultJSON.dropFirst(40)) + "\n\n",
        ])])
        let log = ProgressLog()
        let r = try await client.morph(transcript: "家計簿", appId: "a1", currentVersion: 0, locale: nil) { log.add($0) }
        XCTAssertEqual(r.document.version, 3)
        XCTAssertEqual(r.summary, "できました")
        XCTAssertEqual(log.all, [
            .app(name: "家計簿", tabs: ["ホーム", "集計"], attempt: 1),
            .component(StreamedComponent(attempt: 1, screen: "home", index: 0, id: "t", type: "text", area: "A1:F1", label: "家計簿")),
            .restart(attempt: 2),
            .component(StreamedComponent(attempt: 2, screen: "home", index: 0, id: "t", type: "text", area: "A1:F1")),
        ])
        XCTAssertEqual(ChunkedStubURLProtocol.paths, ["/v1/generate?stream=1"], "1 回で済む")
    }

    func testCutBeforeResultFallsBackToPlainRequest() async throws {
        ChunkedStubURLProtocol.reset([
            .init(chunks: [Self.frame("component", #"{"attempt":1,"screen":"home","component":{"index":0,"type":"list"}}"#)],
                  failAfter: .networkConnectionLost),
            .init(contentType: "application/json", chunks: [Self.resultJSON]),
        ])
        let log = ProgressLog()
        let r = try await client.morph(transcript: "家計簿", appId: "a1", currentVersion: 2, locale: nil) { log.add($0) }
        XCTAssertEqual(r.document.version, 3)
        XCTAssertEqual(ChunkedStubURLProtocol.paths, ["/v1/generate?stream=1", "/v1/generate"], "途中で切れたら stream なしで呼び直す")
        XCTAssertEqual(log.all.last, .restart(attempt: 0), "仮置きを捨ててから戻る")
    }

    func testStreamEndingWithoutResultFallsBack() async throws {
        ChunkedStubURLProtocol.reset([
            .init(chunks: [": ping\n\n", Self.frame("app", #"{"attempt":1,"name":"x"}"#)]),
            .init(contentType: "application/json", chunks: [Self.resultJSON]),
        ])
        let r = try await client.morph(transcript: "x", appId: "a1", currentVersion: 0, locale: nil) { _ in }
        XCTAssertEqual(r.document.version, 3)
        XCTAssertEqual(ChunkedStubURLProtocol.paths.count, 2)
    }

    func testServerIgnoringStreamAnswersJSONOnce() async throws {
        ChunkedStubURLProtocol.reset([.init(contentType: "application/json", chunks: [Self.resultJSON])])
        let r = try await client.morph(transcript: "x", appId: "a1", currentVersion: 0, locale: nil) { _ in }
        XCTAssertEqual(r.document.version, 3)
        XCTAssertEqual(ChunkedStubURLProtocol.paths, ["/v1/generate?stream=1"], "JSON で返ったらそのまま使う(二重に呼ばない)")
    }

    func testUnauthorizedStreamFallsBackToPlainRequest() async throws {
        ChunkedStubURLProtocol.reset([
            .init(status: 401, contentType: "application/json", chunks: [#"{"error":{"code":"unauthorized","message":"x"}}"#]),
            .init(contentType: "application/json", chunks: [Self.resultJSON]),
        ])
        let r = try await client.morph(transcript: "x", appId: "a1", currentVersion: 0, locale: nil) { _ in }
        XCTAssertEqual(r.document.version, 3)
        XCTAssertEqual(ChunkedStubURLProtocol.paths, ["/v1/generate?stream=1", "/v1/generate"], "トークンの取り直しは通常の呼び方に任せる")
    }

    func testErrorEventIsThrownWithoutCallingAgain() async throws {
        ChunkedStubURLProtocol.reset([
            .init(chunks: [Self.frame("error", #"{"error":{"code":"pro_required","message":"有料","status":402}}"#)]),
            .init(contentType: "application/json", chunks: [Self.resultJSON]),
        ])
        do {
            _ = try await client.morph(transcript: "x", appId: "a1", currentVersion: 0, locale: nil) { _ in }
            XCTFail("expected error")
        } catch let e as APIError {
            XCTAssertEqual(e.status, 402)
            XCTAssertEqual(e.code, "pro_required")
        }
        XCTAssertEqual(ChunkedStubURLProtocol.paths, ["/v1/generate?stream=1"], "サーバーが答えたエラーで生成をやり直さない")
    }

    /// 持てるアプリの数の上限(#630)を stream の中で断られた時(ai の handler.go が `event: error` で返す形そのまま)。
    /// 402 の通常の応答と同じく `.appLimit` に分類し、置き換えシートの中身(apps)まで読めること。
    static let appLimitErrorFrame = frame("error", #"""
    {"error":{"code":"app_limit_reached","message":"持てるアプリの数(2 個)に達しました。","status":402,"details":{"resource":"apps","plan":"free","held":2,"limit":2,"base":2,"purchasedSlots":0,"canBuySlot":false,"slotCoins":0,"balance":0,"upgrade":true,"apps":[{"id":"a-old","name":"家計簿","mine":true,"updatedAt":"2026-11-01T00:00:00Z"},{"id":"a-shared","name":"当番表","mine":false,"updatedAt":"2026-11-02T00:00:00Z"}]}}}
    """#)

    func testAppLimitErrorEventClassifiesAsAppLimit() async throws {
        ChunkedStubURLProtocol.reset([.init(chunks: [Self.appLimitErrorFrame])])
        do {
            _ = try await client.morph(transcript: "歩数計作って", appId: "new", currentVersion: 0, locale: nil) { _ in }
            XCTFail("expected error")
        } catch {
            guard case .appLimit(let limit) = FailureKind.classify(error) else {
                return XCTFail("stream の app_limit_reached は .appLimit: \(FailureKind.classify(error)) (\(error))")
            }
            XCTAssertEqual(limit.held, 2)
            XCTAssertEqual(limit.limit, 2)
            XCTAssertTrue(limit.upgrade)
            XCTAssertEqual(limit.apps?.map(\.id), ["a-old", "a-shared"])
            XCTAssertEqual(limit.apps?.map(\.mine), [true, false])
        }
        XCTAssertEqual(ChunkedStubURLProtocol.paths, ["/v1/generate?stream=1"], "上限の断りで生成をやり直さない")
    }

    /// status / message が欠けた `event: error` でも、上限は上限として読む(500 = 混雑にしない)。
    func testAppLimitErrorEventWithoutStatusStillAppLimit() throws {
        let frame = SSEFrame(event: "error", data: #"{"error":{"code":"app_limit_reached","details":{"held":2,"limit":2,"apps":null}}}"#)
        guard case .failure(let e)? = GenerateStreamEvent.decode(frame) else { return XCTFail("error") }
        XCTAssertEqual(e.status, 402)
        guard case .appLimit(let limit) = FailureKind.classify(e) else {
            return XCTFail("status の無い app_limit_reached も .appLimit: \(FailureKind.classify(e))")
        }
        XCTAssertEqual(limit.limit, 2)
        XCTAssertNil(limit.apps, "apps が null なら端末の一覧を使う")
        // status が 500 で来ても code で上限と分かる
        let mislabeled = APIError.server(status: 500, code: AppLimit.code, message: "", details: ["held": .number(2), "limit": .number(2)])
        guard case .appLimit = FailureKind.classify(mislabeled) else { return XCTFail("code で判定する") }
    }

    /// 知らない code の 402 は「言い方を変えて」(rejected)にしない: 言い直しでは直らない。
    func testUnknownPaymentErrorIsNotRejected() {
        let frame = SSEFrame(event: "error", data: #"{"error":{"code":"something_new","message":"x","status":402}}"#)
        guard case .failure(let e)? = GenerateStreamEvent.decode(frame) else { return XCTFail("error") }
        guard case .other = FailureKind.classify(e) else { return XCTFail("402 は .other: \(FailureKind.classify(e))") }
    }

    func testConflictRetriesFromCurrentAfterRestart() async throws {
        ChunkedStubURLProtocol.reset([
            .init(chunks: [Self.frame("error", #"{"error":{"code":"version_conflict","message":"x","status":409}}"#)]),
            .init(chunks: [Self.frame("result", Self.resultJSON)]),
        ])
        let log = ProgressLog()
        let r = try await client.morph(transcript: "x", appId: "a1", currentVersion: 5, locale: nil) { log.add($0) }
        XCTAssertEqual(r.document.version, 3)
        XCTAssertEqual(ChunkedStubURLProtocol.paths, ["/v1/generate?stream=1", "/v1/generate?stream=1"])
        XCTAssertEqual(log.all, [.restart(attempt: 0)])
    }

    func testClientWithoutStreamingStillReturnsResult() async throws {
        let log = ProgressLog()
        let r = try await PlainAI().morph(transcript: "家計簿", appId: "m", currentVersion: 0, locale: nil) { log.add($0) }
        XCTAssertEqual(r.document.version, 3)
        XCTAssertTrue(log.all.isEmpty, "途中経過を出せない実装(モック等)は結果だけ")
    }
}
