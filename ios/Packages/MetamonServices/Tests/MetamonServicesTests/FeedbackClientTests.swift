#if !APP_STORE_REVIEW
import XCTest
@testable import MetamonServices

/// #319: フィードバックの送信。添付は選んだものだけ本文に入り、記録は入らない。
final class FeedbackClientTests: XCTestCase {
    private var savedSession: URLSession!
    private let base = URL(string: "https://appconfig.test/")!
    private var client: HTTPFeedbackClient { HTTPFeedbackClient(baseURL: base, token: StaticTokenProvider("u1")) }
    private let ok = #"{"id":"fb_1","status":"received","createdAt":"2026-11-02T00:00:00Z","attachments":{"screenshot":false,"dsl":false,"device":false}}"#

    override func setUp() {
        super.setUp()
        savedSession = HTTP.session
        HTTP.session = HTTP.makeSession(protocolClasses: [StubURLProtocol.self])
    }

    override func tearDown() {
        HTTP.session = savedSession
        super.tearDown()
    }

    private func lastBody() -> [String: Any]? {
        guard let req = StubURLProtocol.lock.withLock({ StubURLProtocol.requests.last }) else { return nil }
        var data = req.httpBody
        if data == nil, let stream = req.httpBodyStream {
            stream.open()
            defer { stream.close() }
            var buf = [UInt8](repeating: 0, count: 65536)
            var out = Data()
            while stream.hasBytesAvailable {
                let n = stream.read(&buf, maxLength: buf.count)
                if n <= 0 { break }
                out.append(buf, count: n)
            }
            data = out
        }
        return data.flatMap { try? JSONSerialization.jsonObject(with: $0) as? [String: Any] }
    }

    func testMessageOnlyHasNoAttachments() async throws {
        StubURLProtocol.reset([.init(status: 201, body: ok)])
        let r = try await client.send(FeedbackSubmission(kind: .idea, message: "  グラフが欲しい \n"))
        XCTAssertEqual(r.id, "fb_1")
        let req = StubURLProtocol.lock.withLock { StubURLProtocol.requests.last }
        XCTAssertEqual(req?.httpMethod, "POST")
        XCTAssertEqual(req?.url?.path, "/v1/feedback")
        let body = try XCTUnwrap(lastBody())
        XCTAssertEqual(body["message"] as? String, "グラフが欲しい")
        XCTAssertEqual(body["kind"] as? String, "idea")
        XCTAssertEqual(body["consentVersion"] as? String, FeedbackConsent.version)
        XCTAssertNil(body["attachments"], "既定では何も添付しない")
        XCTAssertEqual(Set(body.keys), ["message", "kind", "consentVersion"])
    }

    func testSelectedAttachmentsOnly() async throws {
        StubURLProtocol.reset([.init(status: 201, body: ok)])
        let jpeg = Data([0xFF, 0xD8, 0xFF, 0xE0])
        _ = try await client.send(FeedbackSubmission(message: "x", screenshotJPEG: jpeg,
                                                     app: .init(appId: "a1", dslVersion: 7)))
        let att = try XCTUnwrap(lastBody()?["attachments"] as? [String: Any])
        XCTAssertEqual(att["screenshot"] as? String, jpeg.base64EncodedString())
        XCTAssertEqual((att["app"] as? [String: Any])?["dslVersion"] as? Int, 7)
        XCTAssertNil(att["device"], "選んでいない添付は送らない")
        XCTAssertEqual(Set(att.keys), ["screenshot", "app"], "記録などほかのものは送らない")
    }

    func testRateLimitIsClassified() async {
        StubURLProtocol.reset([.init(status: 429, body: #"{"error":{"code":"rate_limited","message":"m","details":{"scope":"user","limit":5,"retryAfterSec":3600}}}"#,
                                     headers: ["Retry-After": "3600"])])
        do {
            _ = try await client.send(FeedbackSubmission(message: "x"))
            XCTFail("429 must throw")
        } catch {
            guard case .rateLimited = FailureKind.classify(error) else { return XCTFail("\(error)") }
        }
    }
}
#endif
