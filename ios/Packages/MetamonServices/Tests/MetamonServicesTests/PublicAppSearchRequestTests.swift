import XCTest
@testable import MetamonServices

/// 「探す」の検索語は `POST v1/public-apps/search` の本文で送る(URL に載せない)。
final class PublicAppSearchRequestTests: XCTestCase {
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

    func testSearchIsPostWithBody() async throws {
        StubURLProtocol.reset([.init(status: 200, body: #"{"items":[]}"#)])
        let client = HTTPSharingClient(baseURL: base, token: StaticTokenProvider("u1"))
        let items = try await client.searchPublicApps(query: "家計簿 & x=1", sort: "new")
        XCTAssertTrue(items.isEmpty)
        let req = try XCTUnwrap(StubURLProtocol.lock.withLock { StubURLProtocol.requests }.first)
        XCTAssertEqual(req.httpMethod, "POST")
        XCTAssertEqual(req.url?.path, "/v1/public-apps/search")
        XCTAssertNil(req.url?.query, "検索語は URL に載せない")
        XCTAssertEqual(StubURLProtocol.bodyJSON(req), ["q": "家計簿 & x=1", "sort": "new", "pageSize": 50])
    }
}
