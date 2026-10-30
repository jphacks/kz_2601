import XCTest
import MetamonDSL
@testable import MetamonServices

/// #254: 参加者 API のクライアント(パス・メソッド・本文・デコード)と 403 の理由の見分け。
final class SharedMembershipTests: XCTestCase {
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

    func testMembersDecodesListAndRole() async throws {
        StubURLProtocol.reset([.init(status: 200, body: #"""
        {"items":[{"uid":"o","displayName":"","role":"owner","you":false},
                  {"uid":"m1","displayName":"父","role":"member","via":"share","joinedAt":"2026-10-26T00:00:00Z","you":true}],
         "role":"member"}
        """#)])
        let list = try await client.members(appId: "app1")
        XCTAssertEqual(lastRequest?.httpMethod, "GET")
        XCTAssertEqual(lastRequest?.url?.path, "/v1/apps/app1/members")
        XCTAssertEqual(list.role, .member)
        XCTAssertEqual(list.items, [SharedMember(uid: "o", role: .owner), SharedMember(uid: "m1", displayName: "父", role: .member, you: true)])
    }

    func testRemoveAndLeavePaths() async throws {
        StubURLProtocol.reset([.init(status: 204), .init(status: 204)])
        try await client.removeMember(appId: "app1", uid: "uid-2")
        XCTAssertEqual(lastRequest?.httpMethod, "DELETE")
        XCTAssertEqual(lastRequest?.url?.path, "/v1/apps/app1/members/uid-2")
        try await client.leave(appId: "app1")
        XCTAssertEqual(lastRequest?.url?.path, "/v1/apps/app1/members/me")
    }

    func testImportSendsDisplayNameOnlyWhenGiven() async throws {
        let ok = #"{"id":"app1","name":"家計簿","joined":true,"created":true,"member":true}"#
        StubURLProtocol.reset([.init(status: 201, body: ok), .init(status: 201, body: ok)])
        let app = try await client.importShare(token: "tok", displayName: "  父 ")
        XCTAssertEqual(app, InstalledApp(appId: "app1", name: "家計簿", joined: true, member: true))
        XCTAssertEqual(lastRequest?.url?.path, "/v1/shares/tok/import")
        XCTAssertEqual(bodyJSON(lastRequest)?["displayName"] as? String, "父", "前後の空白は落として送る")

        _ = try await client.importShare(token: "tok", displayName: "   ")
        XCTAssertNil(bodyJSON(lastRequest), "空なら本文を送らない(backend は任意)")
    }

    func testPreviewAndSharedScopeProbe() async throws {
        StubURLProtocol.reset([
            .init(status: 200, body: #"{"token":"tok","name":"家族の家計簿","dslVersion":2,"mine":false,"description":"d","components":["list"],"sharedData":true}"#),
            .init(status: 200, body: #"{"appId":"a","version":3,"dsl":{"entities":[{"id":"e1"},{"id":"e2","scope":"shared"}]}}"#),
            .init(status: 200, body: #"{"appId":"b","version":3,"dsl":{"entities":[{"id":"e1","scope":"public"}]}}"#),
        ])
        let preview = try await client.previewShare(token: "tok")
        XCTAssertEqual(preview, ShareLinkPreview(token: "tok", name: "家族の家計簿", description: "d", mine: false, sharedData: true))
        let shared = try await client.hasMembersOnlyData(appId: "a")
        XCTAssertTrue(shared)
        XCTAssertEqual(lastRequest?.url?.path, "/v1/apps/a/dsl")
        let publicOnly = try await client.hasMembersOnlyData(appId: "b")
        XCTAssertFalse(publicOnly, "public だけなら参加者はいない")
    }

    func testDenialReasonFromServerError() async throws {
        StubURLProtocol.reset([.init(status: 403, body: #"{"error":{"code":"forbidden","message":"join this app with a share link to see its shared data","details":{"reason":"not_member"}}}"#)])
        do {
            _ = try await client.members(appId: "app1")
            XCTFail("403 のはず")
        } catch {
            XCTAssertEqual(SharedAccessDenial(error), .notMember)
        }
    }

    func testDenialIgnoresOtherForbidden() {
        // #378 の「作った人だけ」(reason 無し)とは混ぜない
        XCTAssertNil(SharedAccessDenial(APIError.server(status: 403, code: "forbidden", message: "not the author")))
        XCTAssertNil(SharedAccessDenial(APIError.server(status: 400, code: "bad_request", message: "x", details: ["reason": .string("not_member")])))
        XCTAssertEqual(SharedAccessDenial(APIError.server(status: 403, code: "forbidden", message: "", details: ["reason": .string("inviter_left")])), .inviterLeft)
        XCTAssertEqual(SharedAccessDenial(APIError.server(status: 403, code: "forbidden", message: "", details: ["reason": .string("members_full")])), .membersFull)
        XCTAssertNil(SharedAccessDenial(URLError(.notConnectedToInternet)))
    }

    func testInstalledAppMemberDefaultsToFalse() throws {
        let app = try JSONDecoder().decode(InstalledApp.self, from: Data(#"{"id":"a","name":"n","joined":true}"#.utf8))
        XCTAssertFalse(app.member, "#254 より前の backend は member を返さない")
    }
}
