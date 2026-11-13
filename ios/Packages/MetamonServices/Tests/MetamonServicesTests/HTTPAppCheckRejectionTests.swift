import XCTest
@testable import MetamonServices

/// `HTTP.isAppCheckRejection`(401 が App Check で弾かれたかを本文の `error.code` で見分ける)。
/// 判定は本文だけで、再送するかの 401 判定は `exchange` 側。両方を確かめる。
final class HTTPAppCheckRejectionTests: XCTestCase {
    private let base = URL(string: "https://appconfig.example.test/")!
    private var savedSession: URLSession!

    private func envelope(_ code: String) -> String {
        #"{"error":{"code":"\#(code)","message":"m"}}"#
    }

    private func response(_ status: Int, _ body: String) -> HTTP.Response {
        HTTP.Response(status: status, data: Data(body.utf8), headers: [:])
    }

    override func setUp() {
        super.setUp()
        savedSession = HTTP.session
        HTTP.session = HTTP.makeSession(protocolClasses: [StubURLProtocol.self])
        HTTP.appCheck = nil
    }

    override func tearDown() {
        HTTP.session = savedSession
        HTTP.appCheck = nil
        super.tearDown()
    }

    func testDetectionByBodyCode() {
        let rows: [(String, HTTP.Response, Bool)] = [
            ("401 app_check_failed", response(401, envelope("app_check_failed")), true),
            ("403 でも本文のコードが app_check_failed なら true(状態は見ない)", response(403, envelope("app_check_failed")), true),
            ("401 unauthenticated", response(401, envelope("unauthenticated")), false),
            ("403 not_member", response(403, envelope("not_member")), false),
            ("403 forbidden", response(403, envelope("forbidden")), false),
            ("本文が空", response(401, ""), false),
            ("本文が JSON でない", response(401, "<html>Unauthorized</html>"), false),
            ("封筒でない JSON", response(401, #"{"code":"app_check_failed"}"#), false),
        ]
        for (name, r, expected) in rows {
            XCTAssertEqual(HTTP.isAppCheckRejection(r), expected, name)
        }
    }

    func testFailedCodeConstant() {
        XCTAssertEqual(AppCheckHeader.failedCode, "app_check_failed")
    }

    /// 403 は App Check の本文でも取り直して再送しない(再送は 401 だけ)。
    func testForbiddenWithAppCheckBodyIsNotResent() async {
        StubURLProtocol.reset([.init(status: 403, body: envelope("app_check_failed")), .init(status: 200, body: "{}")])
        let token = RefreshingToken()
        do {
            _ = try await HTTP.send("GET", baseURL: base, path: "v1/apps", body: nil, token: token)
            XCTFail("expected 403")
        } catch {
            XCTAssertEqual((error as? APIError)?.status, 403)
        }
        XCTAssertEqual(StubURLProtocol.requests.count, 1)
        XCTAssertEqual(token.refreshCount, 0)
    }

    /// 他の 403 は App Check の失敗として扱わない。
    func testOtherForbiddenIsNotAppCheckFailure() async throws {
        StubURLProtocol.reset([.init(status: 403, body: envelope("not_member"))])
        do {
            _ = try await HTTP.send("GET", baseURL: base, path: "v1/apps", body: nil, token: RefreshingToken())
            XCTFail("expected 403")
        } catch {
            let api = try XCTUnwrap(error as? APIError)
            XCTAssertEqual(api.status, 403)
            XCTAssertFalse(api.isAppCheckFailure)
            XCTAssertNotEqual(FailureKind.classify(error), .appCheckFailed)
        }
    }

    /// App Check の 401 と ID トークン失効の 401 は本文のコードだけで分かれ、ID トークンを取り直すのは後者だけ。
    func testOnly401WithOtherCodeRefreshesIDToken() async throws {
        StubURLProtocol.reset([.init(status: 401, body: envelope("unauthenticated")), .init(status: 200, body: "{}")])
        let token = RefreshingToken()
        _ = try await HTTP.send("GET", baseURL: base, path: "v1/apps", body: nil, token: token)
        XCTAssertEqual(token.refreshCount, 1)
        XCTAssertEqual(StubURLProtocol.requests.count, 2)
    }
}
