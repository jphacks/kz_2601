import XCTest
@testable import MetamonServices

/// 新着の絞り込み(自分の uid)は `POST .../records/query` の本文で送る(URL に載せない)。
/// filter なしの一覧は利用者の値を含まないので GET のまま。
final class RecordsQueryRequestTests: XCTestCase {
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

    private var client: HTTPAppConfigClient { HTTPAppConfigClient(baseURL: base, token: StaticTokenProvider("u1"), cache: nil) }
    private var requests: [URLRequest] { StubURLProtocol.lock.withLock { StubURLProtocol.requests } }

    func testSharedStampsIsPostWithFilterInBody() async throws {
        StubURLProtocol.reset([
            .init(status: 200, body: #"{"items":[{"id":"r1","author":"u2"}],"nextPageToken":"Mg"}"#),
            .init(status: 200, body: #"{"items":[{"id":"r2","author":"u3"}]}"#),
        ])
        let stamps = try await client.sharedStamps(appId: "app-1", entity: "posts", notBy: "me+/&=uid")
        XCTAssertEqual(stamps.map(\.id), ["r1", "r2"])
        XCTAssertEqual(requests.count, 2)
        for req in requests {
            XCTAssertEqual(req.httpMethod, "POST")
            XCTAssertEqual(req.url?.path, "/v1/apps/app-1/entities/posts/records/query")
            XCTAssertNil(req.url?.query, "uid もページも URL に載せない")
        }
        XCTAssertEqual(StubURLProtocol.bodyJSON(requests[0]),
                       ["filterField": "author", "filterOp": "neq", "filterValue": "me+/&=uid", "pageSize": HTTPAppConfigClient.pageSize])
        XCTAssertEqual(StubURLProtocol.bodyJSON(requests[1]),
                       ["filterField": "author", "filterOp": "neq", "filterValue": "me+/&=uid", "pageSize": HTTPAppConfigClient.pageSize, "pageToken": "Mg"])
    }

    func testSharedStampsBadRequestIsEmpty() async throws {
        StubURLProtocol.reset([.init(status: 400, body: #"{"error":{"code":"bad_request","message":"x"}}"#)])
        let stamps = try await client.sharedStamps(appId: "app-1", entity: "private", notBy: "me")
        XCTAssertTrue(stamps.isEmpty)
    }

    func testUnfilteredListStaysGet() async throws {
        StubURLProtocol.reset([.init(status: 200, body: #"{"items":[]}"#)])
        _ = try await client.list(appId: "app-1", entity: "posts")
        let req = try XCTUnwrap(requests.first)
        XCTAssertEqual(req.httpMethod, "GET")
        XCTAssertEqual(req.url?.path, "/v1/apps/app-1/entities/posts/records")
        XCTAssertEqual(req.url?.query, "pageSize=\(HTTPAppConfigClient.pageSize)")
    }
}
