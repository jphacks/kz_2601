#if !APP_STORE_REVIEW
import XCTest
import MetamonDSL
@testable import MetamonServices

/// #321: 通報・ブロックの API クライアント(パス・メソッド・本文・デコード)。
final class ModerationClientTests: XCTestCase {
    private var savedSession: URLSession!
    private let base = URL(string: "https://appconfig.test/")!
    private var client: HTTPSharingClient { HTTPSharingClient(baseURL: base, token: StaticTokenProvider("u1")) }

    override func setUp() {
        super.setUp()
        savedSession = HTTP.session
        HTTP.session = HTTP.makeSession(protocolClasses: [StubURLProtocol.self])
    }

    override func tearDown() {
        HTTP.session = savedSession
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

    func testReportBodies() async throws {
        StubURLProtocol.reset([.init(status: 201, body: #"{"id":"rpt_1","created":true}"#), .init(status: 200, body: #"{"id":"rpt_2","created":false}"#)])
        try await client.report(.record(appId: "app1", entityId: "post", recordId: "r1"), reason: .harassment)
        XCTAssertEqual(lastRequest?.httpMethod, "POST")
        XCTAssertEqual(lastRequest?.url?.path, "/v1/reports")
        let body = bodyJSON(lastRequest)
        XCTAssertEqual(body?["kind"] as? String, "record")
        XCTAssertEqual(body?["appId"] as? String, "app1")
        XCTAssertEqual(body?["entityId"] as? String, "post")
        XCTAssertEqual(body?["recordId"] as? String, "r1")
        XCTAssertEqual(body?["reason"] as? String, "harassment")
        XCTAssertNil(body?["id"])

        try await client.report(.listing(id: "L1"), reason: .spam)
        let listing = bodyJSON(lastRequest)
        XCTAssertEqual(listing?["kind"] as? String, "listing")
        XCTAssertEqual(listing?["id"] as? String, "L1")
    }

    func testBlocks() async throws {
        StubURLProtocol.reset([
            .init(status: 200, body: #"{"items":[{"uid":"u2","name":"はなこ","createdAt":"2026-10-31T00:00:00Z"}]}"#),
            .init(status: 201, body: #"{"uid":"u3","name":"じろう","createdAt":"2026-10-31T00:00:00Z"}"#),
            .init(status: 201, body: #"{"uid":"u4","name":"こた","createdAt":"2026-10-31T00:00:00Z"}"#),
            .init(status: 204),
        ])
        let list = try await client.blockedUsers()
        XCTAssertEqual(lastRequest?.url?.path, "/v1/me/blocks")
        XCTAssertEqual(list, [BlockedUser(uid: "u2", name: "はなこ")])

        let b = try await client.block(uid: "u3", name: "じろう")
        XCTAssertEqual(lastRequest?.httpMethod, "POST")
        XCTAssertEqual(bodyJSON(lastRequest)?["uid"] as? String, "u3")
        XCTAssertEqual(b.uid, "u3")

        let p = try await client.block(listingId: "L1")
        XCTAssertEqual(bodyJSON(lastRequest)?["listingId"] as? String, "L1")
        XCTAssertNil(bodyJSON(lastRequest)?["uid"])
        XCTAssertEqual(p.name, "こた")

        try await client.unblock(uid: "u2")
        XCTAssertEqual(lastRequest?.httpMethod, "DELETE")
        XCTAssertEqual(lastRequest?.url?.path, "/v1/me/blocks/u2")
    }

    func testListingDecodesOperatorHidden() throws {
        let item = try JSONDecoder().decode(PublicAppListing.self, from: Data(#"""
        {"id":"L1","name":"家計簿","description":"","tags":[],"authorName":"こた","components":[],"installs":0,"mine":true,
         "hidden":true,"hiddenReason":"不適切な内容のため"}
        """#.utf8))
        XCTAssertEqual(item.hidden, true)
        XCTAssertEqual(item.hiddenReason, "不適切な内容のため")
        let plain = try JSONDecoder().decode(PublicAppListing.self, from: Data(#"{"id":"L2","name":"x","description":"","tags":[],"authorName":"","components":[],"installs":0,"mine":false}"#.utf8))
        XCTAssertNil(plain.hidden)
    }
}
#endif
