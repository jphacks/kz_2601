import XCTest
@testable import MetamonServices

/// 強制更新の判定の材料(#302 / #981)。iOS と macOS で別のフィールドを引くこと・storeURL の許すスキーム・
/// BuildNumber の比べ方(`1.2.10` と `1.2.9`・壊れた版の文字)。基本の判定は UpdateCheckTests。
final class ClientConfigPlatformTests: XCTestCase {
    private func decode(_ json: String) throws -> ClientConfig {
        try JSONDecoder().decode(ClientConfig.self, from: Data(json.utf8))
    }

    private let both = ClientConfig(minIOSBuild: 10, latestIOSBuild: 20, iosAppStoreUrl: "https://apps.apple.com/app/ios",
                                    minMacOSBuild: 100, latestMacOSBuild: 200, macAppStoreUrl: "https://apps.apple.com/app/mac")

    // MARK: プラットフォームごとの値

    func testMinAndLatestBuildPickPlatformFields() {
        XCTAssertEqual(both.minBuild(.ios), 10)
        XCTAssertEqual(both.latestBuild(.ios), 20)
        XCTAssertEqual(both.minBuild(.macos), 100)
        XCTAssertEqual(both.latestBuild(.macos), 200)
    }

    func testPlatformWithoutValuesIsNil() {
        let iosOnly = ClientConfig(minIOSBuild: 10, latestIOSBuild: 20)
        XCTAssertNil(iosOnly.minBuild(.macos))
        XCTAssertNil(iosOnly.latestBuild(.macos))
        let macOnly = ClientConfig(minMacOSBuild: 5)
        XCTAssertNil(macOnly.minBuild(.ios))
        XCTAssertNil(macOnly.latestBuild(.ios))
    }

    func testEvaluateUsesOwnPlatformOnly() {
        XCTAssertEqual(UpdateRequirement.evaluate(config: both, build: "50", platform: .ios), UpdateRequirement.none, "iOS の最新(20)より新しい")
        XCTAssertEqual(UpdateRequirement.evaluate(config: both, build: "50", platform: .macos), .required(minimum: 100), "Mac は 100 未満")
        XCTAssertEqual(UpdateRequirement.evaluate(config: both, build: "150", platform: .macos), .recommended(latest: 200))
        XCTAssertEqual(UpdateRequirement.evaluate(config: both, build: "9", platform: .ios), .required(minimum: 10))
    }

    func testEvaluateDefaultsToIOS() {
        XCTAssertEqual(UpdateRequirement.evaluate(config: both, build: "9"), .required(minimum: 10))
    }

    func testMacOnlyConfigDoesNotForceIOS() {
        let macOnly = ClientConfig(minMacOSBuild: 100)
        XCTAssertEqual(UpdateRequirement.evaluate(config: macOnly, build: "1", platform: .ios), UpdateRequirement.none, "他の OS の最低版では止めない")
        XCTAssertEqual(UpdateRequirement.evaluate(config: macOnly, build: "1", platform: .macos), .required(minimum: 100))
    }

    func testEvaluateWithDottedBuild() {
        let config = ClientConfig(minIOSBuild: 5, latestIOSBuild: 12)
        XCTAssertEqual(UpdateRequirement.evaluate(config: config, build: "5.0.1"), .recommended(latest: 12), "5.0.1 は最低の 5 以上")
        XCTAssertEqual(UpdateRequirement.evaluate(config: config, build: "4.9.9"), .required(minimum: 5))
        XCTAssertEqual(UpdateRequirement.evaluate(config: config, build: "12.0.0"), UpdateRequirement.none)
    }

    // MARK: decode

    func testDecodeReadsBothPlatformFields() throws {
        let c = try decode(#"{"minIOSBuild":1,"latestIOSBuild":2,"iosAppStoreUrl":"https://x.test/i","minMacOSBuild":3,"latestMacOSBuild":4,"macAppStoreUrl":"https://x.test/m"}"#)
        XCTAssertEqual(c.minIOSBuild, 1)
        XCTAssertEqual(c.latestIOSBuild, 2)
        XCTAssertEqual(c.iosAppStoreUrl, "https://x.test/i")
        XCTAssertEqual(c.minMacOSBuild, 3)
        XCTAssertEqual(c.latestMacOSBuild, 4)
        XCTAssertEqual(c.macAppStoreUrl, "https://x.test/m")
    }

    func testBrokenMacFieldsDoNotBreakIOS() throws {
        let c = try decode(#"{"minIOSBuild":7,"minMacOSBuild":"high","latestMacOSBuild":[],"macAppStoreUrl":5,"notice":"oops"}"#)
        XCTAssertEqual(c.minIOSBuild, 7, "iOS の判定は続ける")
        XCTAssertNil(c.minMacOSBuild)
        XCTAssertNil(c.latestMacOSBuild)
        XCTAssertNil(c.macAppStoreUrl)
        XCTAssertNil(c.notice)
    }

    func testEmptyObjectEnforcesNothing() throws {
        let c = try decode("{}")
        XCTAssertEqual(c, ClientConfig())
        XCTAssertEqual(UpdateRequirement.evaluate(config: c, build: "1", platform: .ios), UpdateRequirement.none)
        XCTAssertEqual(UpdateRequirement.evaluate(config: c, build: "1", platform: .macos), UpdateRequirement.none)
    }

    // MARK: storeURL

    func testStoreURLPicksPlatformField() {
        XCTAssertEqual(both.storeURL(.ios), URL(string: "https://apps.apple.com/app/ios"))
        XCTAssertEqual(both.storeURL(.macos), URL(string: "https://apps.apple.com/app/mac"))
    }

    func testStoreURLAllowsOnlyThatPlatformsSchemes() {
        XCTAssertNotNil(ClientConfig(iosAppStoreUrl: "itms-apps://apps.apple.com/app/id1").storeURL(.ios))
        XCTAssertNil(ClientConfig(iosAppStoreUrl: "macappstore://apps.apple.com/app/id1").storeURL(.ios), "iOS に macappstore は許さない")
        XCTAssertNotNil(ClientConfig(macAppStoreUrl: "macappstore://apps.apple.com/app/id1").storeURL(.macos))
        XCTAssertNil(ClientConfig(macAppStoreUrl: "itms-apps://apps.apple.com/app/id1").storeURL(.macos), "Mac に itms-apps は許さない")
    }

    func testStoreURLRejectsUnsafeOrMissingValues() {
        for raw in ["javascript:alert(1)", "http://apps.apple.com/app/id1", "file:///etc/passwd", "apps.apple.com/app/id1", ""] {
            XCTAssertNil(ClientConfig(iosAppStoreUrl: raw).storeURL(.ios), raw)
            XCTAssertNil(ClientConfig(macAppStoreUrl: raw).storeURL(.macos), raw)
        }
        XCTAssertNil(ClientConfig().storeURL(.ios))
        XCTAssertNil(ClientConfig(macAppStoreUrl: "https://apps.apple.com/app/mac").storeURL(.ios), "他の OS の URL は使わない")
    }

    // MARK: BuildNumber

    func testNumericComponentsAreNotComparedAsStrings() throws {
        let a = try XCTUnwrap(BuildNumber("1.2.10"))
        let b = try XCTUnwrap(BuildNumber("1.2.9"))
        XCTAssertGreaterThan(a, b)
        XCTAssertLessThan(b, a)
        XCTAssertGreaterThan(try XCTUnwrap(BuildNumber("10")), try XCTUnwrap(BuildNumber("9")))
        XCTAssertGreaterThan(try XCTUnwrap(BuildNumber("1.10")), try XCTUnwrap(BuildNumber("1.9.9")))
    }

    func testMissingComponentsCountAsZero() throws {
        XCTAssertEqual(try XCTUnwrap(BuildNumber("1.2")), try XCTUnwrap(BuildNumber("1.2.0.0")))
        XCTAssertLessThan(try XCTUnwrap(BuildNumber("1.2")), try XCTUnwrap(BuildNumber("1.2.0.1")))
        XCTAssertEqual(try XCTUnwrap(BuildNumber("3")), BuildNumber(3))
        XCTAssertEqual(try XCTUnwrap(BuildNumber("0")), try XCTUnwrap(BuildNumber("0.0")))
    }

    func testLeadingZerosAndWhitespace() throws {
        XCTAssertEqual(try XCTUnwrap(BuildNumber("007")), BuildNumber(7))
        XCTAssertEqual(try XCTUnwrap(BuildNumber("  1.2  ")), try XCTUnwrap(BuildNumber("1.2")))
    }

    func testBrokenVersionStringsAreNil() {
        let bad = ["", "   ", ".", "1.", ".1", "1..2", "a", "1.a", "1.2.x", "-1", "1.-2", "1,2", "1 2", "１２", "1e3", "dev", "1.2.3-beta"]
        for s in bad { XCTAssertNil(BuildNumber(s), "「\(s)」は読めない") }
    }

    func testHugeNumberDoesNotCrash() throws {
        XCTAssertNil(BuildNumber("99999999999999999999"), "Int に収まらない数は読めない")
        XCTAssertNil(BuildNumber("1.99999999999999999999"))
    }

    func testEqualityIsAnEquivalenceOnPaddedParts() throws {
        let a = try XCTUnwrap(BuildNumber("2.0"))
        let b = try XCTUnwrap(BuildNumber("2"))
        XCTAssertTrue(a == b)
        XCTAssertFalse(a < b)
        XCTAssertFalse(b < a)
        XCTAssertTrue(a <= b)
        XCTAssertTrue(a >= b)
    }

    func testBrokenBuildFailsOpenOnBothPlatforms() {
        for platform in [ClientConfig.Platform.ios, .macos] {
            XCTAssertEqual(UpdateRequirement.evaluate(config: both, build: "dev", platform: platform), UpdateRequirement.none)
            XCTAssertEqual(UpdateRequirement.evaluate(config: both, build: "", platform: platform), UpdateRequirement.none)
            XCTAssertEqual(UpdateRequirement.evaluate(config: both, build: "1.2.x", platform: platform), UpdateRequirement.none)
        }
    }
}
