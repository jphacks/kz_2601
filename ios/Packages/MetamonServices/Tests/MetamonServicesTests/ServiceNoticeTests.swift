import XCTest
@testable import MetamonServices

/// アプリ内のお知らせ(#333)。デコード・言語・build の範囲・閉じた記録・壊れた notice で版の判定を止めないこと。
final class ServiceNoticeTests: XCTestCase {
    private var savedSession: URLSession!
    private let base = URL(string: "https://appconfig.example.test/")!

    override func setUp() {
        super.setUp()
        savedSession = HTTP.session
        HTTP.session = HTTP.makeSession(protocolClasses: [StubURLProtocol.self])
    }

    override func tearDown() {
        HTTP.session = savedSession
        super.tearDown()
    }

    private func decode(_ json: String) throws -> ClientConfig {
        try JSONDecoder().decode(ClientConfig.self, from: Data(json.utf8))
    }

    func testDecodesNotice() throws {
        let config = try decode(#"{"notice":{"id":"n1","severity":"warning","message":{"ja":"止めています","en":"Paused"},"linkUrl":"https://example.com/s","minIOSBuild":3,"maxIOSBuild":9}}"#)
        XCTAssertEqual(config.notice, ServiceNotice(id: "n1", severity: .warning, message: .init(ja: "止めています", en: "Paused"), linkUrl: "https://example.com/s", minIOSBuild: 3, maxIOSBuild: 9))
    }

    func testUnknownSeverityIsInfo() throws {
        XCTAssertEqual(try decode(#"{"notice":{"id":"n1","severity":"critical","message":{"ja":"x"}}}"#).notice?.severity, .info)
        XCTAssertEqual(try decode(#"{"notice":{"id":"n1","message":{"ja":"x"}}}"#).notice?.severity, .info)
    }

    func testBrokenNoticeKeepsUpdateFields() throws {
        for body in [#"{"minIOSBuild":5,"notice":"oops"}"#, #"{"minIOSBuild":5,"notice":{"id":"n1"}}"#, #"{"minIOSBuild":5,"notice":{"message":{"ja":"x"}}}"#] {
            let config = try decode(body)
            XCTAssertEqual(config.minIOSBuild, 5, body)
            XCTAssertNil(config.notice, body)
        }
    }

    func testTextFollowsLanguage() {
        let n = ServiceNotice(id: "n", message: .init(ja: "日本語", en: "English"))
        XCTAssertEqual(n.text(languageCode: "ja"), "日本語")
        XCTAssertEqual(n.text(languageCode: "en"), "English")
        XCTAssertEqual(n.text(languageCode: "en-US"), "English")
        XCTAssertEqual(n.text(languageCode: "fr"), "日本語", "対応外の言語は日本語")
        XCTAssertEqual(ServiceNotice(id: "n", message: .init(ja: "日本語", en: " ")).text(languageCode: "en"), "日本語", "en が空なら日本語")
    }

    func testLinkMustBeHTTPS() {
        XCTAssertEqual(ServiceNotice(id: "n", message: .init(ja: "x"), linkUrl: "https://example.com").linkURL, URL(string: "https://example.com"))
        for bad in ["http://example.com", "javascript:alert(1)", "tailorit://x", ""] {
            XCTAssertNil(ServiceNotice(id: "n", message: .init(ja: "x"), linkUrl: bad).linkURL, bad)
        }
    }

    func testBuildRange() {
        let n = ServiceNotice(id: "n", message: .init(ja: "x"), minIOSBuild: 5, maxIOSBuild: 9)
        XCTAssertFalse(n.applies(toBuild: "4"))
        XCTAssertTrue(n.applies(toBuild: "5"))
        XCTAssertTrue(n.applies(toBuild: "9"))
        XCTAssertFalse(n.applies(toBuild: "10"))
        XCTAssertFalse(n.applies(toBuild: "dev"), "範囲があって build が読めなければ出さない")
        XCTAssertTrue(ServiceNotice(id: "n", message: .init(ja: "x")).applies(toBuild: "dev"), "範囲なしは全員")
        XCTAssertTrue(ServiceNotice(id: "n", message: .init(ja: "x"), maxIOSBuild: 9).applies(toBuild: "1"))
    }

    func testCheckerFiltersNotice() async {
        let body = #"{"notice":{"id":"n1","message":{"ja":"x"},"maxIOSBuild":9}}"#
        StubURLProtocol.reset([.init(status: 200, body: body)])
        let old = await UpdateChecker(client: HTTPClientConfigClient(baseURL: base), build: "8").check()
        XCTAssertEqual(old.notice?.id, "n1")
        XCTAssertEqual(old.requirement, UpdateRequirement.none)
        StubURLProtocol.reset([.init(status: 200, body: body)])
        let new = await UpdateChecker(client: HTTPClientConfigClient(baseURL: base), build: "10").check()
        XCTAssertNil(new.notice, "対象外の build")
        StubURLProtocol.reset([.init(status: 200, body: #"{"notice":{"id":" ","message":{"ja":"x"}}}"#)])
        let blank = await UpdateChecker(client: HTTPClientConfigClient(baseURL: base), build: "1").check()
        XCTAssertNil(blank.notice, "id が空なら出さない(閉じた記録が付けられない)")
    }

    func testDismissalsPersistPerID() throws {
        let suite = "ServiceNoticeTests.\(UUID().uuidString)"
        let defaults = try XCTUnwrap(UserDefaults(suiteName: suite))
        defer { defaults.removePersistentDomain(forName: suite) }
        let store = NoticeDismissals(defaults: defaults)
        XCTAssertFalse(store.isDismissed("a"))
        store.dismiss("a")
        XCTAssertTrue(NoticeDismissals(defaults: defaults).isDismissed("a"), "起動し直しても覚えている")
        XCTAssertFalse(store.isDismissed("b"), "新しい id は出す")
        for i in 0..<(NoticeDismissals.limit + 5) { store.dismiss("n\(i)") }
        XCTAssertEqual(store.ids.count, NoticeDismissals.limit)
        XCTAssertFalse(store.isDismissed("a"), "古いものから捨てる")
        XCTAssertTrue(store.isDismissed("n\(NoticeDismissals.limit + 4)"))
    }
}
