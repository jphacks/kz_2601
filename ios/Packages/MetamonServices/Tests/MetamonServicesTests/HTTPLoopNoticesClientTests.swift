#if !APP_STORE_REVIEW
import XCTest
@testable import MetamonServices

/// 直ったお知らせ(#374)の HTTP クライアント。要求の道(method・path・pageToken)と本文の形、
/// 応答の読み、失敗の扱いを URLProtocol の作り物で確かめる。decode だけのテストは既にあるので重ねない。
final class HTTPLoopNoticesClientTests: XCTestCase {
    private let base = URL(string: "https://appconfig.example.test/")!
    private var savedSession: URLSession!

    override func setUp() {
        super.setUp()
        savedSession = HTTP.session
        HTTP.session = HTTP.makeSession(protocolClasses: [StubURLProtocol.self])
    }

    override func tearDown() {
        HTTP.session = savedSession
        super.tearDown()
    }

    private struct Token: TokenProvider {
        func idToken() async throws -> String? { "tok-1" }
        func idToken(forceRefresh: Bool) async throws -> String? { "tok-1" }
    }

    private var client: HTTPLoopNoticesClient { HTTPLoopNoticesClient(baseURL: base, token: Token()) }

    private var requests: [URLRequest] { StubURLProtocol.lock.withLock { StubURLProtocol.requests } }

    private func bodyData(_ req: URLRequest) -> Data? {
        if let d = req.httpBody { return d }
        guard let stream = req.httpBodyStream else { return nil }
        stream.open()
        defer { stream.close() }
        var buf = [UInt8](repeating: 0, count: 4096)
        var out = Data()
        while stream.hasBytesAvailable {
            let n = stream.read(&buf, maxLength: buf.count)
            if n <= 0 { break }
            out.append(buf, count: n)
        }
        return out
    }

    private func bodyJSON(_ req: URLRequest) -> [String: Any]? {
        bodyData(req).flatMap { try? JSONSerialization.jsonObject(with: $0) as? [String: Any] }
    }

    private func query(_ req: URLRequest) -> [URLQueryItem] {
        req.url.flatMap { URLComponents(url: $0, resolvingAgainstBaseURL: false)?.queryItems } ?? []
    }

    private let pageJSON = #"{"items":[{"id":"n1","title":"直りました","body":"本文","createdAt":"2026-11-01T00:00:00Z","readAt":null}],"unreadCount":3,"nextPageToken":"next-1"}"#

    // MARK: notices

    func testFirstPageIsGETWithoutQuery() async throws {
        StubURLProtocol.reset([.init(status: 200, body: pageJSON)])
        let page = try await client.notices(pageToken: nil)
        XCTAssertEqual(page.items.map(\.id), ["n1"])
        XCTAssertEqual(page.unreadCount, 3)
        XCTAssertEqual(page.nextPageToken, "next-1")
        XCTAssertTrue(page.items[0].isUnread)

        let req = try XCTUnwrap(requests.first)
        XCTAssertEqual(req.httpMethod, "GET")
        XCTAssertEqual(req.url?.path, "/v1/me/notices")
        XCTAssertNil(req.url?.query, "最初のページに pageToken は付けない")
        XCTAssertNil(bodyData(req), "GET に本文は付けない")
        XCTAssertEqual(req.value(forHTTPHeaderField: "Authorization"), "Bearer tok-1")
    }

    func testNextPageSendsPageToken() async throws {
        StubURLProtocol.reset([.init(status: 200, body: #"{"items":[],"unreadCount":0}"#)])
        let page = try await client.notices(pageToken: "next-1")
        XCTAssertTrue(page.items.isEmpty)
        XCTAssertNil(page.nextPageToken, "最後のページは nextPageToken なし")
        let req = try XCTUnwrap(requests.first)
        XCTAssertEqual(req.httpMethod, "GET")
        XCTAssertEqual(req.url?.path, "/v1/me/notices")
        XCTAssertEqual(query(req), [URLQueryItem(name: "pageToken", value: "next-1")])
    }

    func testPageTokenIsPercentEncodedForQueryValue() async throws {
        StubURLProtocol.reset([.init(status: 200, body: #"{"items":[],"unreadCount":0}"#)])
        _ = try await client.notices(pageToken: "a b&c=d+e/f")
        let req = try XCTUnwrap(requests.first)
        let raw = try XCTUnwrap(req.url?.absoluteString)
        XCTAssertTrue(raw.contains("pageToken=a%20b%26c%3Dd%2Be/f"), "& = + を値の中に漏らさない: \(raw)")
        XCTAssertEqual(query(req), [URLQueryItem(name: "pageToken", value: "a b&c=d+e/f")], "サーバーには元の値で届く")
    }

    func testNoticesDecodeErrorIsThrown() async {
        StubURLProtocol.reset([.init(status: 200, body: "not json")])
        do {
            _ = try await client.notices(pageToken: nil)
            XCTFail("expected error")
        } catch is DecodingError {
            // 読めない応答は握りつぶさない
        } catch {
            XCTFail("DecodingError のはず: \(error)")
        }
    }

    func testNoticesServerErrorIsAPIError() async {
        StubURLProtocol.reset([.init(status: 403, body: #"{"error":{"code":"forbidden","message":"no"}}"#)])
        do {
            _ = try await client.notices(pageToken: nil)
            XCTFail("expected error")
        } catch let e as APIError {
            XCTAssertEqual(e.status, 403)
            XCTAssertEqual(e.code, "forbidden")
        } catch {
            XCTFail("APIError のはず: \(error)")
        }
    }

    // MARK: markRead

    func testMarkReadIsPOSTToReadPathWithoutBody() async throws {
        StubURLProtocol.reset([.init(status: 204, body: "")])
        try await client.markRead(id: "n1")
        XCTAssertEqual(requests.count, 1)
        let req = try XCTUnwrap(requests.first)
        XCTAssertEqual(req.httpMethod, "POST")
        XCTAssertEqual(req.url?.path, "/v1/me/notices/n1/read")
        XCTAssertNil(req.url?.query)
        XCTAssertNil(bodyData(req), "既読に本文は要らない")
        XCTAssertEqual(req.value(forHTTPHeaderField: "Authorization"), "Bearer tok-1")
    }

    func testMarkReadAcceptsEmptyAnd200Responses() async throws {
        StubURLProtocol.reset([.init(status: 200, body: "{}"), .init(status: 200, body: "")])
        try await client.markRead(id: "a")
        try await client.markRead(id: "b")
        XCTAssertEqual(requests.compactMap { $0.url?.path }, ["/v1/me/notices/a/read", "/v1/me/notices/b/read"])
    }

    func testMarkReadFailureIsThrown() async {
        StubURLProtocol.reset([.init(status: 404, body: #"{"error":{"code":"not_found","message":"x"}}"#)])
        do {
            try await client.markRead(id: "gone")
            XCTFail("expected error")
        } catch let e as APIError {
            XCTAssertEqual(e.status, 404)
        } catch {
            XCTFail("APIError のはず: \(error)")
        }
    }

    // MARK: 同意

    func testReadConsentIsGETAndReadsEnabled() async throws {
        StubURLProtocol.reset([.init(status: 200, body: #"{"enabled":true}"#), .init(status: 200, body: #"{"enabled":false}"#)])
        let on = try await client.feedbackFixedConsent()
        let off = try await client.feedbackFixedConsent()
        XCTAssertTrue(on)
        XCTAssertFalse(off)
        for req in requests {
            XCTAssertEqual(req.httpMethod, "GET")
            XCTAssertEqual(req.url?.path, "/v1/me/notify-consent/feedback-fixed")
            XCTAssertNil(bodyData(req))
        }
    }

    func testReadConsentWithoutEnabledIsAnError() async {
        StubURLProtocol.reset([.init(status: 200, body: "{}")])
        do {
            _ = try await client.feedbackFixedConsent()
            XCTFail("expected error")
        } catch is DecodingError {
            // enabled が無い応答は false にせず失敗にする(保存が無いかどうかの扱いはサーバー側)
        } catch {
            XCTFail("DecodingError のはず: \(error)")
        }
    }

    func testSetConsentIsPUTWithEnabledBody() async throws {
        StubURLProtocol.reset([.init(status: 200, body: #"{"enabled":true}"#), .init(status: 204, body: "")])
        try await client.setFeedbackFixedConsent(true)
        try await client.setFeedbackFixedConsent(false)
        let reqs = requests
        XCTAssertEqual(reqs.count, 2)
        for req in reqs {
            XCTAssertEqual(req.httpMethod, "PUT")
            XCTAssertEqual(req.url?.path, "/v1/me/notify-consent/feedback-fixed")
            XCTAssertEqual(req.value(forHTTPHeaderField: "Content-Type"), "application/json")
            XCTAssertEqual(req.value(forHTTPHeaderField: "Authorization"), "Bearer tok-1")
        }
        XCTAssertEqual(bodyJSON(reqs[0])?["enabled"] as? Bool, true)
        XCTAssertEqual(bodyJSON(reqs[1])?["enabled"] as? Bool, false)
        XCTAssertEqual(bodyJSON(reqs[0])?.count, 1, "本文は enabled だけ")
    }

    func testSetConsentFailureIsThrown() async {
        StubURLProtocol.reset([.init(status: 422, body: #"{"error":{"code":"invalid","message":"bad"}}"#)])
        do {
            try await client.setFeedbackFixedConsent(true)
            XCTFail("expected error")
        } catch let e as APIError {
            XCTAssertEqual(e.status, 422)
        } catch {
            XCTFail("APIError のはず: \(error)")
        }
    }

    func testRequestsGoToConfiguredBaseURLHost() async throws {
        StubURLProtocol.reset([.init(status: 200, body: pageJSON), .init(status: 204, body: "")])
        _ = try await client.notices(pageToken: nil)
        try await client.markRead(id: "n1")
        XCTAssertEqual(requests.compactMap { $0.url?.host }, ["appconfig.example.test", "appconfig.example.test"])
    }
}
#endif
