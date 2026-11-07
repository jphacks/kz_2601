import XCTest
import MetamonDSL
@testable import MetamonRenderer

/// M10: 積まれたフォームを戻る前の破棄の確認と、保存バーが「保存しました」を出すかの判定。
@MainActor
final class FormDiscardAndSaveFeedbackTests: XCTestCase {
    private func entity() throws -> Entity {
        try JSONDecoder().decode(Entity.self, from: Data(#"""
        {"id":"memo","name":"メモ","fields":[{"id":"title","name":"タイトル","type":"string"}]}
        """#.utf8))
    }

    private func dirtyDraft() throws -> FormDraft {
        let draft = FormDraft(entity: try entity())
        draft.values["title"] = .string("牛乳")
        return draft
    }

    // MARK: - 破棄の確認

    func testPushedFormAsksOnlyWhenDirty() throws {
        let route = Route(screen: "edit", recordId: "r1", stack: "main")
        let paths = ["main": [Route(screen: "list", stack: "main"), route]]
        let clean = FormDraft(entity: try entity())
        XCTAssertTrue(ScreenView.isPushed(route, paths: paths))
        XCTAssertFalse(ScreenView.guardsDiscard(route: route, paths: paths, form: clean), "何も変えていなければ確認しない")
        XCTAssertTrue(ScreenView.guardsDiscard(route: route, paths: paths, form: try dirtyDraft()))
    }

    func testRootOfMainOrTabDoesNotAsk() throws {
        let draft = try dirtyDraft()
        XCTAssertFalse(ScreenView.guardsDiscard(route: Route(screen: "form", stack: "main"), paths: [:], form: draft))
        XCTAssertFalse(ScreenView.guardsDiscard(route: Route(screen: "form", stack: Route.tabStack("form")), paths: [:], form: draft))
    }

    func testSheetRootAndPushedInSheetAsk() throws {
        let draft = try dirtyDraft()
        let root = Route(screen: "form", stack: "sheet")
        XCTAssertTrue(ScreenView.guardsDiscard(route: root, paths: [:], form: draft))
        XCTAssertFalse(ScreenView.isPushed(root, paths: [:]), "シートの根は「閉じる」で確認する(戻るは無い)")
        let pushed = Route(screen: "edit", recordId: "r1", stack: "sheet")
        XCTAssertTrue(ScreenView.isPushed(pushed, paths: ["sheet": [pushed]]))
    }

    func testReadOnlyAndNoFormDoNotAsk() throws {
        let route = Route(screen: "edit", stack: "main")
        let paths = ["main": [route]]
        let draft = try dirtyDraft()
        draft.isReadOnly = true
        XCTAssertFalse(ScreenView.guardsDiscard(route: route, paths: paths, form: draft))
        XCTAssertFalse(ScreenView.guardsDiscard(route: route, paths: paths, form: nil))
    }

    func testSavedFormNoLongerAsks() throws {
        let route = Route(screen: "edit", stack: "main")
        let draft = try dirtyDraft()
        draft.markSaved(queuedOffline: false)
        XCTAssertFalse(ScreenView.guardsDiscard(route: route, paths: ["main": [route]], form: draft))
    }

    // MARK: - 「保存しました」の判定

    func testNoWriteShowsNothing() throws {
        let draft = try dirtyDraft()
        let before = draft.saveCount
        draft.flagMissingRequired() // 弾かれた・確認待ち・変更なしなど: 書き込みが無い
        XCTAssertNil(FormDraft.saveFeedback(savesBefore: before, form: draft))
        XCTAssertNil(FormDraft.saveFeedback(savesBefore: before, form: nil))
    }

    func testWriteShowsSavedOrQueued() throws {
        let draft = try dirtyDraft()
        var before = draft.saveCount
        draft.markSaved(queuedOffline: false)
        XCTAssertEqual(FormDraft.saveFeedback(savesBefore: before, form: draft), .saved)
        before = draft.saveCount
        draft.values["title"] = .string("卵")
        draft.markSaved(queuedOffline: true)
        XCTAssertEqual(FormDraft.saveFeedback(savesBefore: before, form: draft), .queuedOffline, "オフラインは送信待ちと出す")
        XCTAssertFalse(draft.hasUnsavedChanges)
    }

    func testRefusedAfterSaveRevokesSavedAndRestoresDirty() throws {
        let draft = try dirtyDraft()
        let before = draft.saveCount
        draft.markSaved(queuedOffline: false) // 送信待ちに積んだ時点で数える
        draft.markSaveRefused() // 送った後にサーバーが断った(plan_limit_reached / app_stopped / text_not_allowed)
        XCTAssertNil(FormDraft.saveFeedback(savesBefore: before, form: draft), "断られた書き込みに「保存しました」を出さない")
        XCTAssertTrue(draft.hasUnsavedChanges, "保存されていないので閉じる前に破棄の確認を出す")
        draft.markSaveRefused() // 2 回目は何もしない
        let again = draft.saveCount
        draft.markSaved(queuedOffline: false)
        XCTAssertEqual(FormDraft.saveFeedback(savesBefore: again, form: draft), .saved, "直して保存し直せば出す")
    }

    func testNeedsWrite() throws {
        let fresh = FormDraft(entity: try entity())
        XCTAssertTrue(fresh.needsWrite, "新規は初期値のままでも保存する")
        let editing = FormDraft(entity: try entity(), record: Record(id: "r1", data: ["title": .string("牛乳")]))
        XCTAssertFalse(editing.needsWrite, "既存を何も変えずに保存しても書き込まない")
        editing.values["title"] = .string("卵")
        XCTAssertTrue(editing.needsWrite)
    }
}
