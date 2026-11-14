import XCTest
import MetamonDSL
import MetamonRenderer
import MetamonServices
@testable import Metamon

/// 手で動かす編集モード(#1442)の入口・保存・元に戻す。モック(端末だけで当てる経路)で確かめる。
@MainActor
final class GridEditFlowTests: XCTestCase {
    private func makeRuntime() throws -> AppRuntime {
        let services = BuildConfig.make(environment: .mock)
        let rt = AppRuntime(document: try DSLExamples.document("kakeibo", version: 1), services: services)
        rt.clarifyEnabled = false
        rt.selectedTab = "home"
        return rt
    }

    private func area(_ rt: AppRuntime, _ id: String) -> String? {
        rt.dsl.screen("home")?.components.first { $0.id == id }?.area
    }

    func testMoveSavesNewVersionAndUndoRestores() async throws {
        let rt = try makeRuntime()
        XCTAssertTrue(rt.canStartGridEdit)
        rt.startGridEdit()
        let session = try XCTUnwrap(rt.gridEdit)
        XCTAssertEqual(session.screenId, "home")

        var draft = session.draft
        XCTAssertFalse(draft.place("add", at: GridArea("A10:C11")!), "一覧(A3:F10)と重なる所には置けない")
        XCTAssertTrue(draft.place("add", at: GridArea("A11:C12")!))
        // セッションの draft は内部からしか書けないので、同じ画面で作り直して当てる
        rt.gridEdit = GridEditSession(draft: draft)

        await rt.finishGridEdit()
        XCTAssertNil(rt.gridEdit, "保存したら編集モードを抜ける")
        XCTAssertEqual(rt.document.version, 2)
        XCTAssertEqual(area(rt, "add"), "A11:C12")
        XCTAssertTrue(rt.canUndo)
        XCTAssertEqual(rt.summaryBanner?.action, .undo)

        await rt.undo()
        XCTAssertEqual(area(rt, "add"), "D11:F12", "元に戻すで動かす前の配置に戻る")
    }

    func testCancelKeepsDocument() throws {
        let rt = try makeRuntime()
        rt.startGridEdit()
        var draft = try XCTUnwrap(rt.gridEdit).draft
        XCTAssertTrue(draft.remove("add"))
        rt.gridEdit = GridEditSession(draft: draft)
        rt.cancelGridEdit()
        XCTAssertNil(rt.gridEdit)
        XCTAssertEqual(rt.document.version, 1)
        XCTAssertEqual(area(rt, "add"), "D11:F12")
    }

    func testFinishWithoutChangesDoesNotMakeVersion() async throws {
        let rt = try makeRuntime()
        rt.startGridEdit()
        await rt.finishGridEdit()
        XCTAssertNil(rt.gridEdit)
        XCTAssertEqual(rt.document.version, 1)
    }

    func testFormScreenIsNotEditable() throws {
        let rt = try makeRuntime()
        rt.selectedTab = "home"
        let stack = Route.tabStack("home") // 家計簿はタブのアプリ
        rt.paths[stack] = [Route(screen: "expense_form", stack: stack)]
        XCTAssertFalse(rt.canStartGridEdit, "フォーム画面は中身なりの配置なので対象外")
        rt.startGridEdit()
        XCTAssertNil(rt.gridEdit)
        XCTAssertNotNil(rt.toast)
    }

    /// 編集中に別の版が当たったら(通知から別のアプリを開いた・要約の「元に戻す」等)、前の DSL の下書きを残さない
    func testApplyingAnotherDocumentEndsEditMode() throws {
        let rt = try makeRuntime()
        rt.startGridEdit()
        var draft = try XCTUnwrap(rt.gridEdit).draft
        XCTAssertTrue(draft.place("add", at: GridArea("A11:C12")!))
        rt.gridEdit = GridEditSession(draft: draft)

        var other = try DSLExamples.document("kakeibo", version: 1)
        other.appId = "another-app"
        rt.apply(other, animated: false)
        XCTAssertNil(rt.gridEdit, "別のアプリに前のアプリの下書きを持ち越さない")
        XCTAssertEqual(area(rt, "add"), "D11:F12")
    }

    /// VoiceOver の枠の名前は見出し(無ければ種類)。場所は値として読む。動かした後の読み上げも同じ名前で
    func testFrameLabelUsesTitleOrTypeName() throws {
        let saved = LanguagePreference.shared.setting
        defer { LanguagePreference.shared.setting = saved }
        let rt = try makeRuntime()
        rt.startGridEdit()
        let draft = try XCTUnwrap(rt.gridEdit).draft
        let add = try XCTUnwrap(draft.placed.first { $0.componentId == "add" })
        let summary = try XCTUnwrap(draft.placed.first { $0.componentId == "summary" })
        let recent = try XCTUnwrap(draft.placed.first { $0.componentId == "recent" })

        LanguagePreference.shared.setting = .ja
        XCTAssertEqual(GridEditCopy.label(add), "支出を追加")
        XCTAssertEqual(GridEditCopy.label(recent), "最近の支出")
        XCTAssertEqual(GridEditCopy.label(summary), "まとまり", "見出しの無い部品は種類名")
        XCTAssertEqual(GridEditCopy.moved(add), "支出を追加 を D11:F12 に置きました")

        LanguagePreference.shared.setting = .en
        XCTAssertEqual(GridEditCopy.label(summary), "Group")
        XCTAssertEqual(GridEditCopy.moved(add), "Moved 支出を追加 to D11:F12")
    }

    /// 編集モード中は上の要約・失敗のカードを出さない(編集のバーの「完了」「キャンセル」に被る)
    func testTopCardsHiddenWhileEditing() throws {
        let rt = try makeRuntime()
        XCTAssertTrue(RootView.showsTopCards(gridEditing: rt.gridEdit != nil))
        rt.startGridEdit()
        XCTAssertFalse(RootView.showsTopCards(gridEditing: rt.gridEdit != nil))
        rt.cancelGridEdit()
        XCTAssertTrue(RootView.showsTopCards(gridEditing: rt.gridEdit != nil))
    }
}
