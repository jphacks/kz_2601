import XCTest
@testable import MetamonDSL

/// 審査用ビルド(APP_STORE_REVIEW・#245)の部品の制限。
/// 審査用の条件でも回す: `swift test --filter ReviewBuildTests -Xswiftc -DAPP_STORE_REVIEW`
final class ReviewBuildTests: XCTestCase {
    func testReviewAllowlistExcludesNativeAPIComponents() {
        let allowed = Set(ClientCapabilities.reviewAllowedComponents)
        for t in ClientCapabilities.reviewExcludedComponents { XCTAssertFalse(allowed.contains(t.rawValue), t.rawValue) }
        XCTAssertFalse(allowed.contains(ComponentType.unknown.rawValue))
        for t in ["vstack", "list", "photoField", "videoField", "chart", "stat", "settlement", "calendar", "gridTable", "locationButton", "routeGuide"] {
            XCTAssertTrue(allowed.contains(t), t)
        }
        XCTAssertEqual(ClientCapabilities.reviewDisabledFeatures, ["fieldHealth", "cameraCapture", "sharedData"])
        XCTAssertEqual(ClientCapabilities.supportedFeatures, ["recordRetention"]) // #482: 審査用の版も記録の自動削除を持つ
        XCTAssertFalse(allowed.contains(ComponentType.chatThread.rawValue), "他の人と記録を見る chatThread は外す(#451 G2)")
    }

    func testBuildFlagControlsWhatIsSent() {
        #if APP_STORE_REVIEW
        XCTAssertEqual(ClientCapabilities.allowedComponents, ClientCapabilities.reviewAllowedComponents)
        XCTAssertEqual(ClientCapabilities.disabledFeatures, ClientCapabilities.reviewDisabledFeatures)
        XCTAssertTrue(Set(DSLExamples.names).isDisjoint(with: DSLExamples.reviewExcluded))
        #else
        XCTAssertEqual(ClientCapabilities.allowedComponents, ClientCapabilities.allComponents) // 通常ビルドも全部品を送る(#302)
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
        for name in ["kakeibo", "medicine", "diary_ai", "shopping", "warikan", "training", "walk", "timetable", "water", "drive"] {
            let dsl = try ExamplesFixture.load(name)
            XCTAssertTrue(types(dsl).isDisjoint(with: excluded), "\(name) uses \(types(dsl).intersection(excluded))")
            XCTAssertTrue(dsl.entities.allSatisfy { $0.fields.allSatisfy { $0.health == nil } }, "\(name) uses Field.health")
        }
        // 外した例は本当に外した部品か、共有・公開の記録を使っている(リストの根拠)
        for name in DSLExamples.reviewExcluded {
            let dsl = try ExamplesFixture.load(name)
            XCTAssertTrue(!types(dsl).isDisjoint(with: excluded) || usesSharedData(dsl), name)
        }
    }

    private func usesSharedData(_ dsl: AppDSL) -> Bool {
        dsl.entities.contains { dsl.scope(of: $0.id).isVisibleToOthers }
    }

    /// #451 G2: 審査用ビルドに残す同梱の例は、外した部品も共有・公開の記録も使わない。
    func testRemainingExamplesHaveNoExcludedPartsOrSharedData() throws {
        let excluded = Set(ClientCapabilities.reviewExcludedComponents.map(\.rawValue))
        let remaining = DSLExamples.names.filter { !DSLExamples.reviewExcluded.contains($0) }
        XCTAssertFalse(remaining.isEmpty)
        for name in remaining {
            let dsl = try ExamplesFixture.load(name)
            XCTAssertTrue(types(dsl).isDisjoint(with: excluded), "\(name) uses \(types(dsl).intersection(excluded))")
            XCTAssertFalse(usesSharedData(dsl), "\(name) uses shared / public data")
        }
    }
}
