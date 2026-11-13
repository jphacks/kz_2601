import XCTest
import MetamonDSL

/// Mac はすれ違い通信(nearbyShare の mode passby)を動かせないので、AI に作らせない(#3248・#1955)。
/// nearbyShare 自体(send)は Mac でも使う。iOS の申告は変えない。
final class MacPassbyDeclaration3248Tests: XCTestCase {
    func testMacDisablesPassby() {
        let disabled = ClientCapabilities.disabledFeatures ?? []
        XCTAssertTrue(disabled.contains("nearbyShare.passby"))
        XCTAssertTrue(disabled.contains("fieldHealth"), "前からの値は残す")
        XCTAssertEqual(disabled.filter { $0 == "nearbyShare.passby" }.count, 1, "重ねて送らない")
    }

    func testMacStillRendersNearbyShare() {
        XCTAssertTrue((ClientCapabilities.allowedComponents ?? []).contains(ComponentType.nearbyShare.rawValue), "send は Mac でも使える")
    }

    func testIOSKeepsPassby() {
        XCTAssertNil(ClientCapabilities.disabledFeatures(macOS: false, review: false))
        XCTAssertFalse(ClientCapabilities.disabledFeatures(macOS: false, review: true)?.contains("nearbyShare.passby") ?? false)
    }
}
