import XCTest
import MetamonDSL
@testable import MetamonServices

/// HTTPAppConfigClient の要求(method・パス・JSON の鍵)を URLProtocol の作り物で確かめる。
final class HTTPAppConfigClientBodyTests: XCTestCase {
    private var savedSession: URLSession!
    private var savedApps: [AppSummary] = []
    private let base = URL(string: "https://appconfig.test/")!
    private var client: HTTPAppConfigClient {
        HTTPAppConfigClient(baseURL: base, token: StaticTokenProvider("u1"), cache: nil)
    }

    override func setUp() {
        super.setUp()
        savedSession = HTTP.session
        HTTP.session = HTTP.makeSession(protocolClasses: [StubURLProtocol.self])
        // 変身一覧のキャッシュは実機の保存先と同じなので、前後で戻す
        savedApps = AppListCache.shared.load()
    }

    override func tearDown() {
        HTTP.session = savedSession
        if savedApps.isEmpty { AppListCache.shared.clear() } else { AppListCache.shared.store(savedApps) }
        super.tearDown()
    }

    private var lastRequest: URLRequest? { StubURLProtocol.lock.withLock { StubURLProtocol.requests.last } }

    private func bodyJSON(_ req: URLRequest?) -> [String: Any]? {
        guard let req else { return nil }
        var data = req.httpBody
        if data == nil, let stream = req.httpBodyStream {
            stream.open()
            defer { stream.close() }
            var buf = [UInt8](repeating: 0, count: 4096)
            var out = Data()
            while stream.hasBytesAvailable {
                let n = stream.read(&buf, maxLength: buf.count)
                if n <= 0 { break }
                out.append(buf, count: n)
            }
            data = out
        }
        guard let data, !data.isEmpty else { return nil }
        return try? JSONSerialization.jsonObject(with: data) as? [String: Any]
    }

    private let summaryJSON = #"{"id":"a1","name":"新","currentVersion":2,"status":"active"}"#
    private let recordJSON = #"{"id":"r1","data":{"title":"x"}}"#

    func testRenameSendsPatchWithNameOnly() async throws {
        StubURLProtocol.reset([.init(status: 200, body: summaryJSON)])
        let app = try await client.renameApp(appId: "a1", name: "新")
        XCTAssertEqual(app.name, "新")
        XCTAssertEqual(lastRequest?.httpMethod, "PATCH")
        XCTAssertEqual(lastRequest?.url?.path, "/v1/apps/a1")
        let body = bodyJSON(lastRequest)
        XCTAssertEqual(body?["name"] as? String, "新")
        XCTAssertEqual(body?.count, 1)
    }

    func testDuplicateSendsPostWithName() async throws {
        StubURLProtocol.reset([.init(status: 201, body: summaryJSON)])
        _ = try await client.duplicateApp(appId: "a0", name: "コピー")
        XCTAssertEqual(lastRequest?.httpMethod, "POST")
        XCTAssertEqual(lastRequest?.url?.path, "/v1/apps/a0/duplicate")
        XCTAssertEqual(bodyJSON(lastRequest)?["name"] as? String, "コピー")
    }

    func testReorderSendsPutWithIdsInOrder() async throws {
        StubURLProtocol.reset([.init(status: 204)])
        try await client.reorderApps(ids: ["c", "a", "b"])
        XCTAssertEqual(lastRequest?.httpMethod, "PUT")
        XCTAssertEqual(lastRequest?.url?.path, "/v1/apps/order")
        XCTAssertEqual(bodyJSON(lastRequest)?["ids"] as? [String], ["c", "a", "b"])
    }

    func testCreateSendsDataAndClientIdWithoutAuthorNameKey() async throws {
        StubURLProtocol.reset([.init(status: 201, body: recordJSON)])
        _ = try await client.create(appId: "a1", entity: "memo", data: ["title": .string("x")], clientId: "cid-1")
        XCTAssertEqual(lastRequest?.httpMethod, "POST")
        XCTAssertEqual(lastRequest?.url?.path, "/v1/apps/a1/entities/memo/records")
        let body = bodyJSON(lastRequest)
        XCTAssertEqual(body?["clientId"] as? String, "cid-1")
        XCTAssertEqual((body?["data"] as? [String: Any])?["title"] as? String, "x")
        XCTAssertNil(body?["authorName"], "nil はキーごと送らない")
    }

    func testCreateSendsAuthorNameWhenGiven() async throws {
        StubURLProtocol.reset([.init(status: 201, body: recordJSON)])
        _ = try await client.create(appId: "a1", entity: "memo", data: [:], clientId: "cid-2", authorName: "たろう")
        XCTAssertEqual(bodyJSON(lastRequest)?["authorName"] as? String, "たろう")
    }

    func testCreateWithoutClientIdGeneratesLowercaseUUID() async throws {
        StubURLProtocol.reset([.init(status: 201, body: recordJSON)])
        _ = try await client.create(appId: "a1", entity: "memo", data: [:])
        let id = try XCTUnwrap(bodyJSON(lastRequest)?["clientId"] as? String)
        XCTAssertNotNil(UUID(uuidString: id))
        XCTAssertEqual(id, id.lowercased())
    }

    func testUpdateSendsPatchWithDataOnly() async throws {
        StubURLProtocol.reset([.init(status: 200, body: recordJSON)])
        _ = try await client.update(appId: "a1", entity: "memo", id: "r1", data: ["n": .number(3)])
        XCTAssertEqual(lastRequest?.httpMethod, "PATCH")
        XCTAssertEqual(lastRequest?.url?.path, "/v1/apps/a1/entities/memo/records/r1")
        let body = bodyJSON(lastRequest)
        XCTAssertEqual((body?["data"] as? [String: Any])?["n"] as? Double, 3)
        XCTAssertEqual(body?.count, 1)
    }

    func testDeleteHasNoBody() async throws {
        StubURLProtocol.reset([.init(status: 204)])
        try await client.delete(appId: "a1", entity: "memo", id: "r1")
        XCTAssertEqual(lastRequest?.httpMethod, "DELETE")
        XCTAssertNil(bodyJSON(lastRequest))
    }
}
