import XCTest
@testable import MetamonDSL

/// 審査用ビルド(APP_STORE_REVIEW・#245)の部品の制限。
/// 審査用の条件でも回す: `swift test --filter ReviewBuildTests -Xswiftc -DAPP_STORE_REVIEW`
final class ReviewBuildTests: XCTestCase {
    func testReviewAllowlistExcludesNativeAPIComponents() {
        let allowed = Set(ClientCapabilities.reviewAllowedComponents)
        for t in ClientCapabilities.reviewExcludedComponents { XCTAssertFalse(allowed.contains(t.rawValue), t.rawValue) }
        XCTAssertFalse(allowed.contains(ComponentType.unknown.rawValue))
        for t in ["vstack", "list", "photoField", "videoField", "chart", "stat", "settlement", "calendar", "gridTable", "locationButton"] {
            XCTAssertTrue(allowed.contains(t), t)
        }
        XCTAssertEqual(ClientCapabilities.reviewDisabledFeatures, ["fieldHealth", "cameraCapture"])
    }

    func testBuildFlagControlsWhatIsSent() {
        #if APP_STORE_REVIEW
        XCTAssertEqual(ClientCapabilities.allowedComponents, ClientCapabilities.reviewAllowedComponents)
        XCTAssertEqual(ClientCapabilities.disabledFeatures, ClientCapabilities.reviewDisabledFeatures)
        XCTAssertTrue(Set(DSLExamples.names).isDisjoint(with: DSLExamples.reviewExcluded))
        #else
        XCTAssertNil(ClientCapabilities.allowedComponents) // 通常ビルドは送らない = 今まで通り全部
        XCTAssertNil(ClientCapabilities.disabledFeatures)
        XCTAssertTrue(DSLExamples.reviewExcluded.isSubset(of: Set(DSLExamples.names)))
        #endif
    }

    private func types(_ dsl: AppDSL) -> Set<String> {
        Set(dsl.screens.flatMap { s in s.components.flatMap { [$0.rawType] + $0.children.map(\.rawType) } })
    }

    /// 主要な発話(家計簿・習慣・日記・TODO・割り勘)のモックが返す / 代表する同梱例は、外した部品を使わない。
    func testMainExamplesWorkWithoutExcludedComponents() throws {
        let excluded = Set(ClientCapabilities.reviewExcludedComponents.map(\.rawValue))
        for name in ["kakeibo", "medicine", "diary_ai", "shopping", "warikan", "training", "walk", "timetable", "water"] {
            let dsl = try ExamplesFixture.load(name)
            XCTAssertTrue(types(dsl).isDisjoint(with: excluded), "\(name) uses \(types(dsl).intersection(excluded))")
            XCTAssertTrue(dsl.entities.allSatisfy { $0.fields.allSatisfy { $0.health == nil } }, "\(name) uses Field.health")
        }
        // 外した例は本当に外した部品を使っている(リストの根拠)
        for name in DSLExamples.reviewExcluded {
            XCTAssertFalse(types(try ExamplesFixture.load(name)).isDisjoint(with: excluded), name)
        }
    }
}
