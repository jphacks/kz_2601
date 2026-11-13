import XCTest
import MetamonDSL
@testable import MetamonServices

/// 生成のストリーム(#636)の「restart」と「使えない時は普通の生成に落ちる」(StreamUnavailable)。
/// 基本の流れは GenerateStreamTests。ここは restart の読み方・落ちる / 落ちない応答の境目を URLProtocol の作り物で確かめる。
final class GenerateStreamRestartTests: XCTestCase {
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

    private struct Token: TokenProvider {
        func idToken() async throws -> String? { "tok" }
        func idToken(forceRefresh: Bool) async throws -> String? { "tok" }
    }

    private final class Log: @unchecked Sendable {
        private let lock = NSLock()
        private var items: [GenerateProgress] = []
        func add(_ p: GenerateProgress) { lock.withLock { items.append(p) } }
        var all: [GenerateProgress] { lock.withLock { items } }
    }

    private var client: HTTPAIClient { HTTPAIClient(baseURL: base, token: Token(), cache: nil) }
    private var resultJSON: String { GenerateStreamTests.resultJSON }

    private func frame(_ event: String, _ data: String) -> String { "event: \(event)\ndata: \(data)\n\n" }

    private func morph(_ log: Log = Log()) async throws -> MorphResult {
        try await client.morph(transcript: "x", appId: "a1", currentVersion: 0, locale: nil) { log.add($0) }
    }

    private func component(_ attempt: Int, _ id: String) -> String {
        frame("component", #"{"attempt":\#(attempt),"screen":"home","component":{"index":0,"id":"\#(id)","type":"text"}}"#)
    }

    // MARK: restart event

    func testRestartEventWithoutAttemptIsAttemptZero() {
        for data in ["{}", #"{"attempt":null}"#, "{壊れた", #"{"attempt":"2"}"#] {
            guard case .progress(.restart(let attempt))? = GenerateStreamEvent.decode(SSEFrame(event: "restart", data: data)) else {
                return XCTFail("restart は壊れていても読み飛ばさない: \(data)")
            }
            XCTAssertEqual(attempt, 0, data)
        }
    }

    func testRestartEventsAreDeliveredInOrderBetweenComponents() async throws {
        ChunkedStubURLProtocol.reset([.init(chunks: [
            component(1, "a"),
            frame("restart", #"{"attempt":2}"#),
            component(2, "b"),
            frame("restart", #"{"attempt":3}"#),
            frame("result", resultJSON),
        ])])
        let log = Log()
        _ = try await morph(log)
        XCTAssertEqual(log.all.map(Self.label), ["component 1 a", "restart 2", "component 2 b", "restart 3"],
                       "restart のたびに利用側が前の部品を捨てられるよう、順番どおりに渡す")
    }

    func testRestartEventDoesNotCallAgain() async throws {
        ChunkedStubURLProtocol.reset([.init(chunks: [frame("restart", #"{"attempt":2}"#), frame("result", resultJSON)])])
        _ = try await morph()
        XCTAssertEqual(ChunkedStubURLProtocol.paths, ["/v1/generate?stream=1"], "restart はサーバー内のやり直し。通信はやり直さない")
    }

    func testFallbackReportsRestartZeroAfterEarlierProgress() async throws {
        ChunkedStubURLProtocol.reset([
            .init(chunks: [frame("app", #"{"attempt":1,"name":"家計簿"}"#), component(1, "a")], failAfter: .networkConnectionLost),
            .init(contentType: "application/json", chunks: [resultJSON]),
        ])
        let log = Log()
        let r = try await morph(log)
        XCTAssertEqual(r.document.version, 3)
        // 切断の前に何件届くかは URLProtocol の配信の都合。順番(最後が restart 0・1 回だけ)を見る
        let labels = log.all.map(Self.label)
        XCTAssertEqual(labels.last, "restart 0", "仮置きを出した後に切れたら、最後に restart 0")
        XCTAssertEqual(labels.filter { $0 == "restart 0" }.count, 1)
    }

    // MARK: StreamUnavailable(落ちる応答)

    func testTooManyRequestsFallsBackToPlainRequest() async throws {
        ChunkedStubURLProtocol.reset([
            .init(status: 429, contentType: "application/json", chunks: [#"{"error":{"code":"rate_limited","message":"x"}}"#]),
            .init(contentType: "application/json", chunks: [resultJSON]),
        ])
        let log = Log()
        let r = try await morph(log)
        XCTAssertEqual(r.document.version, 3)
        XCTAssertEqual(ChunkedStubURLProtocol.paths, ["/v1/generate?stream=1", "/v1/generate"], "429 の再送は通常の呼び方に任せる")
        XCTAssertEqual(log.all.map(Self.label), ["restart 0"])
    }

    func testFallbackRequestFailureIsThrownNotRetriedAgain() async throws {
        ChunkedStubURLProtocol.reset([
            .init(status: 401, contentType: "application/json", chunks: [#"{"error":{"code":"unauthorized","message":"x"}}"#]),
            .init(status: 500, contentType: "application/json", chunks: [#"{"error":{"code":"internal","message":"boom"}}"#]),
        ])
        do {
            _ = try await morph()
            XCTFail("expected error")
        } catch let e as APIError {
            XCTAssertEqual(e.status, 500)
        }
        XCTAssertEqual(ChunkedStubURLProtocol.paths.count, 2, "普通の生成も 1 回だけ")
    }

    // MARK: StreamUnavailable にしない応答(そのまま扱う)

    func testServerErrorBodyIsThrownWithoutFallback() async throws {
        ChunkedStubURLProtocol.reset([
            .init(status: 503, contentType: "application/json", chunks: [#"{"error":{"code":"upstream_busy","message":"混雑"}}"#]),
            .init(contentType: "application/json", chunks: [resultJSON]),
        ])
        do {
            _ = try await morph()
            XCTFail("expected error")
        } catch let e as APIError {
            XCTAssertEqual(e.status, 503)
            XCTAssertEqual(e.code, "upstream_busy")
        }
        XCTAssertEqual(ChunkedStubURLProtocol.paths, ["/v1/generate?stream=1"], "サーバーが答えた 5xx で生成をやり直さない")
    }

    func testClientErrorBodyIsThrownWithoutFallback() async throws {
        ChunkedStubURLProtocol.reset([
            .init(status: 422, contentType: "application/json", chunks: [#"{"error":{"code":"invalid_request","message":"空"}}"#]),
        ])
        do {
            _ = try await morph()
            XCTFail("expected error")
        } catch let e as APIError {
            XCTAssertEqual(e.status, 422)
        }
        XCTAssertEqual(ChunkedStubURLProtocol.paths, ["/v1/generate?stream=1"])
    }

    func testOKResponseWithWrongContentTypeIsReadAsPlainBody() async throws {
        ChunkedStubURLProtocol.reset([.init(contentType: "text/plain", chunks: [resultJSON])])
        let log = Log()
        let r = try await morph(log)
        XCTAssertEqual(r.document.version, 3)
        XCTAssertEqual(ChunkedStubURLProtocol.paths, ["/v1/generate?stream=1"], "event-stream でない 200 は本文をそのまま結果として読む")
        XCTAssertTrue(log.all.isEmpty)
    }

    func testUnreadableResultIsThrownWithoutFallback() async throws {
        ChunkedStubURLProtocol.reset([
            .init(chunks: [frame("result", #"{"nope":true}"#)]),
            .init(contentType: "application/json", chunks: [resultJSON]),
        ])
        do {
            _ = try await morph()
            XCTFail("expected error")
        } catch is DecodingError {
            // 結果は届いたが読めない: 通常の呼び方でやり直さない(生成を二重に走らせない)
        }
        XCTAssertEqual(ChunkedStubURLProtocol.paths, ["/v1/generate?stream=1"])
    }

    func testBrokenErrorEventBecomesHTTP500WithoutFallback() async throws {
        ChunkedStubURLProtocol.reset([
            .init(chunks: [frame("error", "{壊れた")]),
            .init(contentType: "application/json", chunks: [resultJSON]),
        ])
        do {
            _ = try await morph()
            XCTFail("expected error")
        } catch let e as APIError {
            XCTAssertEqual(e.status, 500)
        }
        XCTAssertEqual(ChunkedStubURLProtocol.paths, ["/v1/generate?stream=1"])
    }

    func testUnterminatedResultFrameFallsBack() async throws {
        // 区切りの空行が来る前に終わった result は捨てる(SSE の決まり)。普通の呼び方で結果を取り直す
        ChunkedStubURLProtocol.reset([
            .init(chunks: ["event: result\ndata: " + resultJSON]),
            .init(contentType: "application/json", chunks: [resultJSON]),
        ])
        let log = Log()
        _ = try await morph(log)
        XCTAssertEqual(ChunkedStubURLProtocol.paths, ["/v1/generate?stream=1", "/v1/generate"])
        XCTAssertEqual(log.all.map(Self.label), ["restart 0"])
    }

    // MARK: -

    private static func label(_ p: GenerateProgress) -> String {
        switch p {
        case .app(let name, _, _): return "app \(name)"
        case .component(let c): return "component \(c.attempt) \(c.id ?? "")"
        case .restart(let attempt): return "restart \(attempt)"
        }
    }
}
