import XCTest
import MetamonDSL
@testable import MetamonRenderer
import MetamonServices
@testable import Metamon

/// 画面の並びの編集(#2323): 入口(表示メニュー・ツールバー)の有効 / 無効と、保存(端末で当てる・封筒の書き換え)。
@MainActor
final class MacGridEditTests: XCTestCase {
    private func kakeiboURL() throws -> URL {
        try XCTUnwrap(Bundle(for: MacAppRuntime.self).url(forResource: "kakeibo", withExtension: "json"))
    }

    private func runtime(saving: Bool = true) throws -> MacAppRuntime {
        let rt = MacAppRuntime(appId: "test-mac-\(UUID().uuidString.lowercased())", dsl: try AppDSL.load(contentsOf: try kakeiboURL()),
                               ai: MockAIClient(delay: .zero), isAIConsentGranted: { false })
        if saving { rt.onSaveGridEdit = { _ in .saved } }
        return rt
    }

    private func navigate(_ rt: MacAppRuntime, _ json: String) async throws {
        let action = try JSONDecoder().decode(Action.self, from: Data(json.utf8))
        await rt.run(action, ActionContext(route: Route(screen: "home", stack: Route.tabStack("home"))))
    }

    // MARK: - 入口の有効 / 無効

    func testEnabledOnGridScreen() throws {
        let rt = try runtime()
        XCTAssertEqual(rt.currentScreen, "home")
        XCTAssertTrue(rt.canStartGridEdit, "area 付きの部品がある根の画面は編集できる")
    }

    func testDisabledWithoutSaveDestination() throws {
        XCTAssertFalse(try runtime(saving: false).canStartGridEdit, "保存先が無ければ入口を出さない")
    }

    func testDisabledOnFormScreen() async throws {
        let rt = try runtime()
        try await navigate(rt, #"{"type":"navigate","screen":"expense_form","presentation":"push"}"#)
        XCTAssertEqual(rt.currentScreen, "expense_form")
        XCTAssertFalse(rt.canStartGridEdit, "フォームの画面は配置が中身なりなので対象外")
    }

    func testDisabledWhileSheetIsOpen() async throws {
        let rt = try runtime()
        try await navigate(rt, #"{"type":"navigate","screen":"stats","presentation":"sheet"}"#)
        XCTAssertNotNil(rt.sheet)
        XCTAssertFalse(rt.canStartGridEdit, "シートの間は根の画面を編集しない")
    }

    func testDisabledWhileEditing() throws {
        let rt = try runtime()
        rt.startGridEdit()
        XCTAssertEqual(rt.gridEdit?.screenId, "home")
        XCTAssertFalse(rt.canStartGridEdit, "編集中は二重に入らない")
        XCTAssertTrue(rt.isInteractingForHotReload, "編集中は他の端末の版で作り直さない")
        rt.cancelGridEdit()
        XCTAssertNil(rt.gridEdit)
        XCTAssertTrue(rt.canStartGridEdit)
    }

    // MARK: - 完了

    func testFinishWithoutChangesJustCloses() async throws {
        let rt = try runtime()
        var calls = 0
        rt.onSaveGridEdit = { _ in calls += 1; return .saved }
        rt.startGridEdit()
        await rt.finishGridEdit()
        XCTAssertNil(rt.gridEdit)
        XCTAssertEqual(calls, 0, "何も変えていなければ保存しない")
    }

    func testFailedSaveKeepsDraft() async throws {
        let rt = try runtime()
        rt.onSaveGridEdit = { _ in .failed }
        rt.startGridEdit()
        let session = try XCTUnwrap(rt.gridEdit)
        XCTAssertTrue(session.place("add", at: try XCTUnwrap(GridArea("A11:C12"))))
        await rt.finishGridEdit()
        XCTAssertNotNil(rt.gridEdit, "保存できなければ下書きを残す")
        XCTAssertEqual(rt.toast, L10n.tr("配置を保存できませんでした"))
    }

    // MARK: - 保存

    func testLocalSaveBumpsVersionAndRewritesEnvelope() async throws {
        let dslData = try Data(contentsOf: try kakeiboURL())
        let dsl = try AppDSL.load(contentsOf: try kakeiboURL())
        let base = AppDocument(appId: "kakeibo", version: 3, dsl: dsl)
        let envelope = try XCTUnwrap(DocumentEnvelope.make(appId: "kakeibo", version: 3, dsl: dslData))
        let index = try XCTUnwrap(dsl.screens.firstIndex { $0.id == "home" })
        var draft = GridEditDraft(screen: dsl.screens[index], screenIndex: index)
        let moved = try XCTUnwrap(GridArea("A11:C12"))
        XCTAssertTrue(draft.place("add", at: moved))

        let saved = try await MacGridEditSave.save(draft, base: base, baseEnvelope: envelope, server: nil)
        XCTAssertEqual(saved.document.version, 4)
        XCTAssertEqual(saved.document.dsl.screen("home")?.components.first { $0.id == "add" }?.area, "A11:C12")

        let reread = try AppDocument.decode(try XCTUnwrap(saved.envelope))
        XCTAssertEqual(reread.version, 4, "端末に残す封筒も同じ版")
        XCTAssertEqual(reread.dsl.screen("home")?.components.first { $0.id == "add" }?.area, "A11:C12")
        XCTAssertEqual(reread.dsl.screen("home")?.components.first { $0.id == "recent" }?.area, "A3:F10", "動かしていない部品はそのまま")
    }

    func testEnvelopeRewriteRemovesDeletedComponent() throws {
        let dslData = try Data(contentsOf: try kakeiboURL())
        let dsl = try AppDSL.load(contentsOf: try kakeiboURL())
        let index = try XCTUnwrap(dsl.screens.firstIndex { $0.id == "home" })
        var draft = GridEditDraft(screen: dsl.screens[index], screenIndex: index)
        XCTAssertTrue(draft.remove("summary"))
        let envelope = try XCTUnwrap(DocumentEnvelope.make(appId: "kakeibo", version: 1, dsl: dslData))
        let rewritten = try XCTUnwrap(MacGridEditSave.applied(draft, to: envelope, version: 2))
        let ids = try AppDocument.decode(rewritten).dsl.screen("home")?.components.map(\.id)
        XCTAssertEqual(ids, ["add", "recent"])
    }

    func testEnvelopeRewriteRejectsMismatchedScreen() throws {
        let dsl = try AppDSL.load(contentsOf: try kakeiboURL())
        let index = try XCTUnwrap(dsl.screens.firstIndex { $0.id == "home" })
        let draft = GridEditDraft(screen: dsl.screens[index], screenIndex: index)
        let other = try XCTUnwrap(DocumentEnvelope.make(appId: "x", version: 1,
                                                        dsl: Data(#"{"app":{"name":"x"},"screens":[{"id":"other","components":[]}]}"#.utf8)))
        XCTAssertNil(MacGridEditSave.applied(draft, to: other, version: 2), "画面の並びが違う封筒には当てない")
    }
}
