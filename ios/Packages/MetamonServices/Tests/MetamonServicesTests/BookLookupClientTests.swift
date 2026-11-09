import XCTest
import MetamonDSL
@testable import MetamonServices

/// lookupBook: 本の情報は appconfig の `v1/lookup/book` だけに聞く(外部へ直接つながらない)。
final class BookLookupClientTests: XCTestCase {
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

    func testRequestAndDecode() async throws {
        let body = #"{"isbn":"9784780802047","title":"おにぎりレシピ101","authors":"山田玲子／著","publisher":"ポット出版","publishedDate":"2014-04-08","source":{"name":"openBD","url":"https://openbd.jp/","license":"l"},"fetchedAt":"2026-11-08T03:00:00Z"}"#
        StubURLProtocol.reset([.init(status: 200, body: body)])
        let client = HTTPBookLookupClient(baseURL: base, token: StaticTokenProvider("u1"))
        let b = try await client.lookup(isbn: "9784780802047")
        XCTAssertEqual(b.title, "おにぎりレシピ101")
        XCTAssertEqual(b.source.name, "openBD")
        let req = try XCTUnwrap(StubURLProtocol.lock.withLock { StubURLProtocol.requests }.first)
        XCTAssertEqual(req.httpMethod, "POST")
        XCTAssertEqual(req.url?.host(), "appconfig.test")
        XCTAssertEqual(req.url?.path, "/v1/lookup/book")
        XCTAssertNil(req.url?.query, "ISBN は URL に載せない")
        XCTAssertEqual(req.value(forHTTPHeaderField: "Content-Type"), "application/json")
        XCTAssertEqual(StubURLProtocol.bodyJSON(req), ["isbn": "9784780802047"])
    }

    func testDisabledIsBookLookupOff() async throws {
        StubURLProtocol.reset([.init(status: 503, body: #"{"error":{"code":"upstream_unavailable","message":"本の情報の取得は今使えません","details":{"reason":"disabled"}}}"#)])
        let client = HTTPBookLookupClient(baseURL: base, token: StaticTokenProvider("u1"))
        do {
            _ = try await client.lookup(isbn: "9784780802047")
            XCTFail("503 は throw")
        } catch let e as APIError {
            XCTAssertTrue(e.isBookLookupOff)
        }
        XCTAssertFalse(APIError.server(status: 503, code: "upstream_unavailable", message: "つながらない").isBookLookupOff)
        XCTAssertFalse(APIError.server(status: 404, code: "not_found", message: "", details: ["reason": .string("disabled")]).isBookLookupOff)
    }

    func testNotFoundIsAPIError404() async throws {
        StubURLProtocol.reset([.init(status: 404, body: #"{"error":{"code":"not_found","message":"この ISBN の本は見つかりませんでした"}}"#)])
        let client = HTTPBookLookupClient(baseURL: base, token: StaticTokenProvider("u1"))
        do {
            _ = try await client.lookup(isbn: "9780000000002")
            XCTFail("404 は throw")
        } catch let e as APIError {
            XCTAssertEqual(e.status, 404)
        }
    }
}
