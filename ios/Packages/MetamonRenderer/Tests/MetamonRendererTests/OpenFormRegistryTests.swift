import XCTest
import MetamonDSL
@testable import MetamonRenderer

/// #943: Mac の Esc・⌘W・アプリの切り替えの前に、今も出ているフォームに保存していない入力があるかを見る。
@MainActor
final class OpenFormRegistryTests: XCTestCase {
    private func entity() throws -> Entity {
        try JSONDecoder().decode(Entity.self, from: Data(#"""
        {"id":"memo","name":"メモ","fields":[{"id":"title","name":"タイトル","type":"string"}]}
        """#.utf8))
    }

    private func dsl() throws -> AppDSL {
        try JSONDecoder().decode(AppDSL.self, from: Data(#"""
        {"schemaVersion":1,"app":{"name":"t"},"initialScreen":"home","entities":[],
         "screens":[{"id":"home","title":"ホーム","components":[]},{"id":"edit","title":"編集","components":[]}]}
        """#.utf8))
    }

    private let sheetRoute = Route(screen: "edit", stack: "sheet")

    func testOnlyUnsavedFormsCount() throws {
        let registry = OpenFormRegistry()
        let draft = FormDraft(entity: try entity())
        registry.track(draft, at: sheetRoute)
        XCTAssertTrue(registry.unsavedRoutes { _ in true }.isEmpty, "開いただけでは確認しない")
        draft.values["title"] = "牛乳"
        XCTAssertEqual(registry.unsavedRoutes { _ in true }, [sheetRoute])
        draft.markClean() // 保存した
        XCTAssertTrue(registry.unsavedRoutes { _ in true }.isEmpty, "保存の後は確認しない")
    }

    func testFilterAndUntrack() throws {
        let registry = OpenFormRegistry()
        let draft = FormDraft(entity: try entity())
        registry.track(draft, at: sheetRoute)
        draft.values["title"] = "牛乳"
        XCTAssertTrue(registry.unsavedRoutes { $0.stack == "main" }.isEmpty, "閉じる画面に無いフォームは数えない")
        registry.untrack(draft)
        XCTAssertTrue(registry.unsavedRoutes { _ in true }.isEmpty)
    }

    func testReleasedFormIsForgotten() throws {
        let registry = OpenFormRegistry()
        var draft: FormDraft? = FormDraft(entity: try entity())
        registry.track(draft!, at: sheetRoute)
        draft?.values["title"] = "牛乳"
        draft = nil // 画面が消えた
        XCTAssertTrue(registry.unsavedRoutes { _ in true }.isEmpty)
    }

    func testReadOnlyNeverAsks() throws {
        let registry = OpenFormRegistry()
        let draft = FormDraft(entity: try entity(), record: Record(id: "r1", data: ["title": "牛乳"], author: "other"))
        draft.isReadOnly = true
        registry.track(draft, at: sheetRoute)
        draft.values["title"] = "変更"
        XCTAssertTrue(registry.unsavedRoutes { _ in true }.isEmpty)
    }

    func testIsPresented() throws {
        let dsl = try dsl()
        let pushed = Route(screen: "edit", recordId: "r1")
        var state = NavigationState(paths: ["main": [pushed]], sheet: sheetRoute)
        XCTAssertTrue(NavigationResolver.isPresented(Route(screen: "home"), in: state, dsl: dsl), "根は常に出ている")
        XCTAssertTrue(NavigationResolver.isPresented(pushed, in: state, dsl: dsl))
        XCTAssertTrue(NavigationResolver.isPresented(sheetRoute, in: state, dsl: dsl))
        state = NavigationResolver.close(state, pushed)
        state = NavigationResolver.close(state, sheetRoute)
        XCTAssertFalse(NavigationResolver.isPresented(pushed, in: state, dsl: dsl), "戻った画面は出ていない")
        XCTAssertFalse(NavigationResolver.isPresented(sheetRoute, in: state, dsl: dsl), "閉じたシートは出ていない")
    }
}
