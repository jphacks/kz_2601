import XCTest
import MetamonServices
@testable import Metamon

/// 複製(#1285)も作る時と同じ持てるアプリの数の上限(#630)を通る。iOS はサーバーの 402 で断るが、Mac は端末の数で確かめる。
@MainActor
final class MacDuplicateAppLimitTests: XCTestCase {
    /// 端末に残さない一覧に、モックの AI で作ったアプリを `count` 個足す
    private func makeLibrary(created count: Int) async throws -> MacAppLibrary {
        let library = MacAppLibrary(store: nil)
        for i in 0..<count {
            let result = try await MockAIClient(delay: .zero).morph(transcript: "家計簿のアプリ", appId: "mac-\(i)", currentVersion: 0)
            var document = result.document
            document.appId = "mac-\(i)"
            library.apply(document, envelope: result.envelope, created: true)
        }
        return library
    }

    func testUnderLimitDuplicates() async throws {
        let library = try await makeLibrary(created: 1)
        XCTAssertNil(library.limitForAdding(planApps: MacPlanApps.free))
        XCTAssertNotNil(library.duplicate("mac-0"))
        XCTAssertEqual(library.createdIds.count, 2)
    }

    func testAtLimitReturnsLimitForReplaceSheet() async throws {
        let library = try await makeLibrary(created: MacPlanApps.free)
        let limit = try XCTUnwrap(library.limitForAdding(planApps: MacPlanApps.free))
        XCTAssertEqual(limit.held, 2)
        XCTAssertEqual(limit.limit, 2)
        XCTAssertTrue(limit.upgrade) // Pro にすると増える
        XCTAssertNil(limit.apps) // 並べるのはこの Mac で作ったアプリ(置き換えシートの context)

        // 置き換えシートには作ったアプリが並び、1 つ消すと閉じる(やり直せる)
        let summaries = library.createdIds.map { AppSummary(id: $0, name: library.generated[$0]?.dsl.app.name ?? "", currentVersion: 2) }
        let context = MacAppReplaceContext(apps: summaries, current: "mac-0", delete: { library.delete($0) })
        let plan = MacAppLimitSheet.makePlan(limit, context: context)
        XCTAssertEqual(Set(plan.rows.map(\.id)), ["mac-0", "mac-1"])
        XCTAssertEqual(plan.rows.last?.id, "mac-0") // 使用中は最後
        XCTAssertNil(plan.removing("mac-1"))
    }

    func testFreeingOneLetsDuplicateRetry() async throws {
        let library = try await makeLibrary(created: MacPlanApps.free)
        XCTAssertNotNil(library.limitForAdding(planApps: MacPlanApps.free))
        library.delete("mac-1")
        XCTAssertNil(library.limitForAdding(planApps: MacPlanApps.free))
        XCTAssertNotNil(library.duplicate("mac-0"))
        XCTAssertEqual(library.createdIds.count, 2)
    }

    func testProLimitAndZeroMeansUnlimited() async throws {
        let library = try await makeLibrary(created: MacPlanApps.free)
        XCTAssertNil(library.limitForAdding(planApps: MacPlanApps.pro))
        XCTAssertNil(library.limitForAdding(planApps: 0)) // 0 は数えない(サーバーと同じ)
        let full = try await makeLibrary(created: MacPlanApps.pro)
        let limit = try XCTUnwrap(full.limitForAdding(planApps: MacPlanApps.pro))
        XCTAssertFalse(limit.upgrade)
    }
}
