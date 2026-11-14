import XCTest
import MetamonDSL
@testable import MetamonServices

/// 手で動かした配置(#1442)は `PATCH /v1/apps/{appId}/dsl` に If-Match 付きで送り、返った最新 DSL を使う。
final class PatchDSLRequestTests: XCTestCase {
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

    func testPatchSendsIfMatchAndManualSource() async throws {
        let envelope = #"{"appId":"app-1","version":8,"dsl":{"schemaVersion":1,"app":{"name":"x","voiceEntry":"none"},"initialScreen":"home","screens":[{"id":"home","title":"","components":[{"type":"text","text":"a","area":"A1:C3"}]}]}}"#
        StubURLProtocol.reset([.init(status: 201, body: envelope)])
        let doc = try await client.patchDSL(
            appId: "app-1", baseVersion: 7,
            patch: [JSONPatchOp(op: "replace", path: "/screens/0/components/0/area", value: .string("A1:C3"))],
            summary: "配置を変えました")
        XCTAssertEqual(doc.version, 8)
        let req = try XCTUnwrap(StubURLProtocol.lock.withLock { StubURLProtocol.requests.first })
        XCTAssertEqual(req.httpMethod, "PATCH")
        XCTAssertEqual(req.url?.path, "/v1/apps/app-1/dsl")
        XCTAssertEqual(req.value(forHTTPHeaderField: "If-Match"), "\"7\"")
        let body = try XCTUnwrap(StubURLProtocol.bodyJSON(req))
        XCTAssertEqual(body["patch"] as? [[String: String]], [["op": "replace", "path": "/screens/0/components/0/area", "value": "A1:C3"]])
        XCTAssertEqual(body["source"] as? [String: String], ["kind": "manual", "mode": "patch", "summary": "配置を変えました"])
    }

    func testConflictIsThrown() async {
        StubURLProtocol.reset([.init(status: 409, body: #"{"error":{"code":"version_conflict","message":"x"}}"#)])
        do {
            _ = try await client.patchDSL(appId: "app-1", baseVersion: 7, patch: [], summary: "")
            XCTFail("expected an error")
        } catch let error as APIError {
            XCTAssertEqual(error.status, 409)
        } catch {
            XCTFail("unexpected \(error)")
        }
    }
}
