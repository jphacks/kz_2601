import XCTest
import MetamonServices
@testable import Metamon

/// Mac の強制アップデート(#981): Mac の版のフィールドで判定し、iOS の値には反応しない・fail open・ストアの URL
@MainActor
final class MacUpdateGateTests: XCTestCase {
    private struct ConfigClient: ClientConfigClient {
        var config: ClientConfig
        func fetch() async throws -> ClientConfig { config }
    }

    private struct FailingClient: ClientConfigClient {
        func fetch() async throws -> ClientConfig { throw URLError(.notConnectedToInternet) }
    }

    private func gate(_ config: ClientConfig?, build: String, configured: URL? = nil) async -> MacUpdateGate {
        let client: any ClientConfigClient = config.map { ConfigClient(config: $0) } ?? FailingClient()
        let gate = MacUpdateGate(configuredStoreURL: configured)
        gate.apply(await UpdateChecker(client: client, build: build, platform: .macos).check())
        return gate
    }

    /// backend の `minMacOSBuild` / `latestMacOSBuild` / `macAppStoreUrl`(#981)を読む
    func testDecodesMacFields() throws {
        let json = #"{"minIOSBuild":12,"minMacOSBuild":40,"latestMacOSBuild":42,"macAppStoreUrl":"https://apps.apple.com/app/id1"}"#
        let config = try JSONDecoder().decode(ClientConfig.self, from: Data(json.utf8))
        XCTAssertEqual(config, ClientConfig(minIOSBuild: 12, minMacOSBuild: 40, latestMacOSBuild: 42, macAppStoreUrl: "https://apps.apple.com/app/id1"))
        XCTAssertEqual(UpdateRequirement.evaluate(config: config, build: "41", platform: .macos), .recommended(latest: 42))
        XCTAssertEqual(UpdateRequirement.evaluate(config: config, build: "41"), UpdateRequirement.none, "既定は iOS の値")
    }

    func testBelowMinimumBlocks() async {
        let gate = await gate(ClientConfig(minMacOSBuild: 40), build: "39")
        XCTAssertEqual(gate.requirement, .required(minimum: 40))
        XCTAssertTrue(gate.isBlocking)
        XCTAssertFalse(gate.showsNotice, "閉じられない画面の間はお知らせを出さない")
    }

    func testAtMinimumDoesNotBlock() async {
        let gate = await gate(ClientConfig(minMacOSBuild: 40), build: "40")
        XCTAssertFalse(gate.isBlocking)
        XCTAssertEqual(gate.requirement, UpdateRequirement.none)
    }

    func testIOSValuesAreIgnoredOnMac() async {
        let gate = await gate(ClientConfig(minIOSBuild: 999, latestIOSBuild: 999), build: "1")
        XCTAssertEqual(gate.requirement, UpdateRequirement.none, "iOS の最低対応版で Mac を止めない")
    }

    func testRecommendedIsDismissible() async {
        let gate = await gate(ClientConfig(minMacOSBuild: 10, latestMacOSBuild: 50), build: "20")
        XCTAssertFalse(gate.isBlocking)
        XCTAssertTrue(gate.showsNotice)
        gate.dismissNotice()
        XCTAssertFalse(gate.showsNotice, "同じ latest は 1 起動で 1 回")
    }

    func testFailOpen() async {
        let failing = await gate(nil, build: "1")
        XCTAssertFalse(failing.isBlocking)
        let noClient = MacUpdateGate(configuredStoreURL: nil)
        noClient.apply(await UpdateChecker(client: nil, build: "1", platform: .macos).check())
        XCTAssertFalse(noClient.isBlocking)
    }

    func testStoreURLPrefersServerThenInfoPlistThenSearch() async {
        let server = await gate(ClientConfig(minMacOSBuild: 40, macAppStoreUrl: "https://apps.apple.com/app/id1"), build: "1",
                                configured: URL(string: "https://apps.apple.com/app/id2"))
        XCTAssertEqual(server.storeURL.absoluteString, "macappstore://apps.apple.com/app/id1", "Mac App Store のアプリで開く")

        let plist = await gate(ClientConfig(iosAppStoreUrl: "https://apps.apple.com/app/id9", minMacOSBuild: 40), build: "1",
                               configured: URL(string: "https://apps.apple.com/app/id2"))
        XCTAssertEqual(plist.storeURL.absoluteString, "macappstore://apps.apple.com/app/id2", "iOS の URL は使わない")

        let bad = await gate(ClientConfig(minMacOSBuild: 40, macAppStoreUrl: "itms-apps://apps.apple.com/app/id1"), build: "1")
        XCTAssertEqual(bad.storeURL, MacAppStoreLink.search, "Mac で開けないスキームは使わない")
        XCTAssertEqual(MacAppStoreLink.search.scheme, "macappstore")
    }
}
