import XCTest
@testable import MetamonServices

/// お知らせの対象の OS(#3244・backend #2964 の `platforms`)。無し = 全部の OS・ios だけ・macos だけ。
final class ServiceNoticePlatforms3244Tests: XCTestCase {
    private struct FakeClient: ClientConfigClient {
        let config: ClientConfig
        func fetch() async throws -> ClientConfig { config }
    }

    private func decode(_ json: String) throws -> ServiceNotice? {
        try JSONDecoder().decode(ClientConfig.self, from: Data(json.utf8)).notice
    }

    private func shown(_ notice: ServiceNotice, on platform: ClientConfig.Platform) async -> Bool {
        let checker = UpdateChecker(client: FakeClient(config: ClientConfig(notice: notice)), build: "1", platform: platform)
        return await checker.check().notice != nil
    }

    func testNoPlatformsShowsEverywhere() async throws {
        let n = try XCTUnwrap(try decode(#"{"notice":{"id":"n1","message":{"ja":"x"}}}"#))
        XCTAssertNil(n.platforms)
        XCTAssertTrue(n.applies(to: .ios))
        XCTAssertTrue(n.applies(to: .macos))
        let ios = await shown(n, on: .ios)
        let mac = await shown(n, on: .macos)
        XCTAssertTrue(ios)
        XCTAssertTrue(mac)
        XCTAssertTrue(ServiceNotice(id: "n", message: .init(ja: "x"), platforms: []).applies(to: .macos), "空 = 指定なし")
    }

    func testIOSOnly() async throws {
        let n = try XCTUnwrap(try decode(#"{"notice":{"id":"n1","message":{"ja":"x"},"platforms":[" iOS ","android"]}}"#))
        XCTAssertEqual(n.platforms, ["ios", "android"], "大文字・空白をならす")
        XCTAssertTrue(n.applies(to: .ios))
        XCTAssertFalse(n.applies(to: .macos))
        let ios = await shown(n, on: .ios)
        let mac = await shown(n, on: .macos)
        XCTAssertTrue(ios)
        XCTAssertFalse(mac, "Mac には出さない")
    }

    func testMacOSOnly() async throws {
        let n = try XCTUnwrap(try decode(#"{"notice":{"id":"n1","message":{"ja":"x"},"platforms":["macos"]}}"#))
        XCTAssertFalse(n.applies(to: .ios))
        XCTAssertTrue(n.applies(to: .macos))
        let ios = await shown(n, on: .ios)
        let mac = await shown(n, on: .macos)
        XCTAssertFalse(ios, "iPhone には出さない")
        XCTAssertTrue(mac)
    }

    func testBrokenPlatformsKeepsNotice() throws {
        let n = try XCTUnwrap(try decode(#"{"notice":{"id":"n1","message":{"ja":"x"},"platforms":"ios"}}"#), "notice ごと捨てない")
        XCTAssertNil(n.platforms)
        XCTAssertTrue(n.applies(to: .macos))
    }
}
