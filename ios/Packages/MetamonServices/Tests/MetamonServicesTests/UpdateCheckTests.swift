import XCTest
import MetamonDSL
@testable import MetamonServices

/// 最低対応版と強制アップデート(#302)。比較・fail open・ヘッダ。
final class UpdateCheckTests: XCTestCase {
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

    // MARK: - 比較

    func testBuildNumberComparison() throws {
        func b(_ s: String) throws -> BuildNumber { try XCTUnwrap(BuildNumber(s), s) }
        XCTAssertLessThan(try b("9"), try b("10"), "文字列ではなく数で比べる")
        XCTAssertLessThan(try b("1.2"), try b("1.10"))
        XCTAssertLessThan(try b("1.2.3"), try b("1.3"))
        XCTAssertEqual(try b("1.2"), try b("1.2.0"), "足りない桁は 0")
        XCTAssertEqual(try b(" 12 "), BuildNumber(12))
        for bad in ["", "abc", "1..2", "1.x", "-1"] { XCTAssertNil(BuildNumber(bad), bad) }
    }

    func testEvaluate() {
        let config = ClientConfig(minIOSBuild: 10, latestIOSBuild: 15)
        XCTAssertEqual(UpdateRequirement.evaluate(config: config, build: "9"), .required(minimum: 10))
        XCTAssertEqual(UpdateRequirement.evaluate(config: config, build: "10"), .recommended(latest: 15), "MIN ちょうどは使える")
        XCTAssertEqual(UpdateRequirement.evaluate(config: config, build: "15"), UpdateRequirement.none)
        XCTAssertEqual(UpdateRequirement.evaluate(config: config, build: "120"), UpdateRequirement.none)
        XCTAssertEqual(UpdateRequirement.evaluate(config: ClientConfig(minIOSBuild: 10), build: "11"), UpdateRequirement.none)
        XCTAssertEqual(UpdateRequirement.evaluate(config: ClientConfig(latestIOSBuild: 15), build: "3"), .recommended(latest: 15))
    }

    func testEvaluateFailsOpen() {
        XCTAssertEqual(UpdateRequirement.evaluate(config: nil, build: "1"), UpdateRequirement.none, "設定が取れない")
        XCTAssertEqual(UpdateRequirement.evaluate(config: ClientConfig(), build: "1"), UpdateRequirement.none, "未設定 = 強制しない")
        XCTAssertEqual(UpdateRequirement.evaluate(config: ClientConfig(minIOSBuild: 10), build: "dev"), UpdateRequirement.none, "自分の build が読めない")
        XCTAssertEqual(UpdateRequirement.evaluate(config: ClientConfig(minIOSBuild: 0, latestIOSBuild: -1), build: "1"), UpdateRequirement.none)
    }

    // MARK: - 取得(fail open)

    private let base = URL(string: "https://appconfig.example.test/")!

    func testCheckReadsServerConfig() async throws {
        StubURLProtocol.reset([.init(status: 200, body: #"{"minIOSBuild":5,"latestIOSBuild":8,"iosAppStoreUrl":"https://apps.apple.com/app/id1"}"#)])
        let result = await UpdateChecker(client: HTTPClientConfigClient(baseURL: base), build: "4").check()
        XCTAssertEqual(result.requirement, .required(minimum: 5))
        XCTAssertEqual(result.appStoreURL, URL(string: "https://apps.apple.com/app/id1"))
        let req = try XCTUnwrap(StubURLProtocol.requests.first)
        XCTAssertEqual(req.url?.path, "/v1/client-config")
        XCTAssertNil(req.value(forHTTPHeaderField: "Authorization"), "サインイン前にも読むのでトークンは付けない")
    }

    func testCheckFailsOpenOnErrors() async {
        let cases: [StubURLProtocol.Canned] = [
            .init(status: 503, body: #"{"error":{"code":"unavailable","message":"down"}}"#),
            .init(status: 404, body: ""),
            .init(status: 200, body: "not json"),
            .init(status: 200, body: #"{"minIOSBuild":"high"}"#),
        ]
        for canned in cases {
            StubURLProtocol.reset([canned])
            let result = await UpdateChecker(client: HTTPClientConfigClient(baseURL: base), build: "1").check()
            XCTAssertEqual(result.requirement, UpdateRequirement.none, "\(canned.status) \(canned.body)")
        }
        let noClient = await UpdateChecker(client: nil, build: "1").check()
        XCTAssertEqual(noClient.requirement, UpdateRequirement.none, "モック(appconfig なし)")
    }

    func testUnsafeStoreURLIsIgnored() async {
        StubURLProtocol.reset([.init(status: 200, body: #"{"minIOSBuild":5,"iosAppStoreUrl":"javascript:alert(1)"}"#)])
        let result = await UpdateChecker(client: HTTPClientConfigClient(baseURL: base), build: "4").check()
        XCTAssertEqual(result.requirement, .required(minimum: 5))
        XCTAssertNil(result.appStoreURL)
    }

    // MARK: - 版のヘッダ

    func testEveryRequestCarriesClientVersionHeaders() async throws {
        StubURLProtocol.reset([.init(status: 200, body: "{}")])
        _ = try await HTTP.send("GET", baseURL: base, path: "v1/apps", body: nil, token: NoAuthTokenProvider())
        let req = try XCTUnwrap(StubURLProtocol.requests.first)
        XCTAssertEqual(req.value(forHTTPHeaderField: "X-Client-Platform"), "ios")
        XCTAssertEqual(req.value(forHTTPHeaderField: "X-Client-Build"), ClientInfo.current.build)
        XCTAssertEqual(req.value(forHTTPHeaderField: "X-Client-Version"), ClientInfo.current.version)
        XCTAssertEqual(req.value(forHTTPHeaderField: "X-Client-DSL-Version"), String(ClientCapabilities.supportedSchemaVersion))
    }

    /// 通常ビルドも描ける全部品を送る(#302): backend はここに無い(後の版で足された)部品を出さない。
    func testGenerateBodySendsEveryRenderableComponent() throws {
        let body = HTTPAIClient.GenerateBody(appId: "a", baseVersion: nil, transcript: "t")
        let sent = try XCTUnwrap(body.allowedComponents)
        XCTAssertFalse(sent.contains(ComponentType.unknown.rawValue))
        #if !APP_STORE_REVIEW
        XCTAssertEqual(Set(sent), Set(ComponentType.allCases.filter { $0 != .unknown }.map(\.rawValue)))
        #endif
        let chat = HTTPAIClient.ChatBody(appId: "a", messages: [])
        XCTAssertEqual(chat.allowedComponents, body.allowedComponents, "generate と chat は同じ一覧")
    }
}
