import XCTest
import MetamonDSL
import MetamonRenderer
import MetamonServices
@testable import Metamon

/// Mac の申告(#925)と Renderer の登録(#947)の一致。表は docs/decisions/363-desktop-strategy.md「部品の対応(Mac の列)」
@MainActor
final class MacPlatformDeclarationTests: XCTestCase {
    private var unavailable: Set<String> { Set(ClientCapabilities.macUnavailableComponents.map(\.rawValue)) }

    func testPlatformHeaderIsMacOS() {
        XCTAssertEqual(ClientCapabilities.platform, "macos")
        XCTAssertEqual(ClientInfo(version: "1.0.0", build: "1").headers["X-Client-Platform"], "macos")
    }

    func testMacDeclarationIsFixed() {
        XCTAssertEqual(unavailable, ["healthStat", "sensorValue", "locationTracker", "dashcam"])
        let allowed = Set(ClientCapabilities.allowedComponents ?? [])
        XCTAssertTrue(allowed.isDisjoint(with: unavailable), "Mac で描けない部品を AI に作らせない")
        XCTAssertFalse(allowed.contains(ComponentType.unknown.rawValue))
        let disabled = ClientCapabilities.disabledFeatures ?? []
        XCTAssertTrue(disabled.contains("fieldHealth"))
        // 撮影は Mac のカメラで撮れる(#1043)。審査用の版だけ外す
        XCTAssertFalse(disabled.contains("cameraCapture"))
        XCTAssertEqual(ClientCapabilities.disabledFeatures(macOS: true, review: true)?.contains("cameraCapture"), true)
    }

    /// iOS の値は変えない(同じ関数を iOS の引数で呼ぶ)
    func testIOSDeclarationUnchanged() {
        XCTAssertEqual(ClientCapabilities.allowedComponents(macOS: false, review: false), ClientCapabilities.allComponents)
        XCTAssertEqual(ClientCapabilities.allowedComponents(macOS: false, review: true), ClientCapabilities.reviewAllowedComponents)
        XCTAssertNil(ClientCapabilities.disabledFeatures(macOS: false, review: false))
        XCTAssertEqual(ClientCapabilities.disabledFeatures(macOS: false, review: true), ClientCapabilities.reviewDisabledFeatures)
    }

    /// 使えないと申告した部品はすべて「この Mac では使えません」の枠、使えると申告した部品はすべて登録済みで枠ではない
    func testRegistryMatchesDeclaration() {
        let registry = ComponentRegistry.standard
        XCTAssertEqual(registry.unavailableTypes, unavailable)
        for type in unavailable {
            XCTAssertNotNil(registry.builder(for: type), "枠で受ける: \(type)")
        }
        for type in ClientCapabilities.allowedComponents ?? [] {
            XCTAssertNotNil(registry.builder(for: type), "申告したのに登録が無い: \(type)")
            XCTAssertFalse(registry.unavailableTypes.contains(type), "申告したのに枠: \(type)")
        }
    }

    /// 全部品が「使える」か「Mac で使えない」(か審査用で外した)のどれかに入っている
    func testEveryComponentIsClassified() {
        let allowed = Set(ClientCapabilities.allowedComponents ?? [])
        let review = Set(ClientCapabilities.reviewExcludedComponents.map(\.rawValue))
        for type in ClientCapabilities.allComponents {
            XCTAssertTrue(allowed.contains(type) || unavailable.contains(type) || review.contains(type), type)
        }
    }
}
