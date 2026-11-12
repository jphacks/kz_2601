import XCTest
import MetamonDSL
@testable import MetamonRenderer
import MetamonServices
@testable import Metamon

/// 画面マップから記録を選んで詳細・編集の画面を開く(#1322)。家計簿の例: ホームの一覧の行 → expense_form。
@MainActor
final class MacScreenMapRecordTests: XCTestCase {
    private func kakeibo() throws -> AppDSL {
        let url = try XCTUnwrap(Bundle(for: MacAppRuntime.self).url(forResource: "kakeibo", withExtension: "json"))
        return try AppDSL.load(contentsOf: url)
    }

    func testRecordEntityIsTheListOpeningTheScreen() throws {
        let dsl = try kakeibo()
        XCTAssertEqual(MacScreenMapView.recordEntity(opening: "expense_form", in: dsl), "expense")
        XCTAssertNil(MacScreenMapView.recordEntity(opening: "stats", in: dsl), "一覧の行から開かない画面は記録を選ばない")
        XCTAssertNil(MacScreenMapView.recordEntity(opening: "home", in: dsl))
    }

    func testRevealOpensTheScreenWithTheRecord() throws {
        let rt = MacAppRuntime(appId: "test-map-\(UUID().uuidString.lowercased())", dsl: try kakeibo(),
                               ai: MockAIClient(delay: .zero), isAIConsentGranted: { false })
        rt.reveal(screen: "expense_form", recordId: "r1")
        let top = NavigationResolver.top(rt.navigationState, dsl: rt.dsl)
        XCTAssertEqual(top.screen, "expense_form")
        XCTAssertEqual(top.recordId, "r1")
        XCTAssertTrue(rt.canGoBack, "根(ホーム)から開くので戻れる")

        rt.reveal(screen: "expense_form")
        XCTAssertNil(NavigationResolver.top(rt.navigationState, dsl: rt.dsl).recordId, "記録を選ばなければ今まで通り新規")
    }
}
