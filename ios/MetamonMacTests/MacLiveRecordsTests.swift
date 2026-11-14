import XCTest
import MetamonDSL
import MetamonServices
@testable import Metamon

/// 作ったアプリの記録をサーバーにつなぐ(#1284 の続き): サーバーにあるアプリの印と、記録の置き場の選び方
@MainActor
final class MacLiveRecordsTests: XCTestCase {
    private var suite: String!
    private var defaults: UserDefaults!

    override func setUp() {
        super.setUp()
        suite = "mac-live-records-\(UUID().uuidString)"
        defaults = UserDefaults(suiteName: suite)
    }

    override func tearDown() {
        defaults.removePersistentDomain(forName: suite)
        super.tearDown()
    }

    private func document(_ id: String, version: Int = 1) throws -> AppDocument {
        let url = try XCTUnwrap(Bundle(for: MacAppRuntime.self).url(forResource: "kakeibo", withExtension: "json"))
        return AppDocument(appId: id, version: version, dsl: try AppDSL.load(contentsOf: url))
    }

    func testLiveResultMarksAppOnServerAndPersists() throws {
        let library = MacAppLibrary(store: nil, defaults: defaults)
        library.apply(try document("mac-live"), created: true, onServer: true)
        library.apply(try document("mac-mock"), created: true)
        XCTAssertTrue(library.isOnServer("mac-live"))
        XCTAssertFalse(library.isOnServer("mac-mock"), "モックの AI で作ったアプリは端末だけ")

        let reopened = MacAppLibrary(store: nil, defaults: defaults)
        XCTAssertTrue(reopened.isOnServer("mac-live"), "印は次の起動にも残る")
    }

    func testDeleteClearsServerMark() throws {
        let library = MacAppLibrary(store: nil, defaults: defaults)
        library.apply(try document("mac-live"), created: true, onServer: true)
        library.delete("mac-live")
        XCTAssertFalse(library.isOnServer("mac-live"))
        XCTAssertFalse(MacAppLibrary(store: nil, defaults: defaults).isOnServer("mac-live"))
    }

    func testMockServicesHaveNoAppConfig() {
        let services = MacServices.make(environment: ["METAMON_MOCK": "1",
                                                      "METAMON_APPCONFIG_BASE_URL": "https://appconfig.example.com"])
        XCTAssertNil(services.appConfig)
        XCTAssertNil(services.appConfigURL)
        XCTAssertFalse(services.keepsAppsOnServer)
    }

    func testLiveServicesBuildHTTPAppConfig() {
        let services = MacServices(auth: MockAuthService(signedIn: true), ai: MockAIClient(), feedback: MockFeedbackClient(),
                                   isLive: true, appConfigURL: URL(string: "https://appconfig.example.com/"))
        XCTAssertTrue(services.appConfig is HTTPAppConfigClient)
        XCTAssertTrue(services.keepsAppsOnServer)
        let offline = MacServices(auth: MockAuthService(signedIn: true), ai: MockAIClient(), feedback: MockFeedbackClient(),
                                  isLive: false, appConfigURL: URL(string: "https://appconfig.example.com/"))
        XCTAssertNil(offline.appConfig, "モックの時は URL があってもつながない")
    }

    /// サーバーの記録: 書いた行がサーバー(ここではメモリ上の appconfig)に届く
    func testServerRecordSyncWritesToServer() async throws {
        let root = FileManager.default.temporaryDirectory.appendingPathComponent("mac-vault-\(UUID().uuidString)")
        defer { try? FileManager.default.removeItem(at: root) }
        let server = InMemoryAppConfigClient()
        let engine = MacRecordVault.recordSync(appId: "mac-live", server: server, root: root)
        engine.start()
        defer { engine.stop() }
        _ = await engine.save(appId: "mac-live", entity: "expense", recordId: nil, data: ["amount": .number(500)])
        for _ in 0..<100 {
            if try await !server.list(appId: "mac-live", entity: "expense").isEmpty { break }
            try await Task.sleep(for: .milliseconds(20))
        }
        let rows = try await server.list(appId: "mac-live", entity: "expense")
        XCTAssertEqual(rows.count, 1)
    }
}
