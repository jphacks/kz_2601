import XCTest
import MetamonDSL
@testable import MetamonRenderer

/// M10: シートのフォームを閉じる前に破棄を確認するための「保存していない入力があるか」。
@MainActor
final class FormDraftUnsavedTests: XCTestCase {
    private func entity() throws -> Entity {
        try JSONDecoder().decode(Entity.self, from: Data(#"""
        {"id":"memo","name":"メモ","fields":[{"id":"title","name":"タイトル","type":"string"},{"id":"amount","name":"金額","type":"number"}]}
        """#.utf8))
    }

    func testNewFormIsCleanUntilInput() throws {
        let draft = FormDraft(entity: try entity())
        XCTAssertFalse(draft.hasUnsavedChanges, "開いただけでは確認しない")
        draft.values["title"] = "牛乳"
        XCTAssertTrue(draft.hasUnsavedChanges)
        draft.values["title"] = ""
        XCTAssertFalse(draft.hasUnsavedChanges, "空に戻したら入力なし")
    }

    func testEditedRecordAndSave() throws {
        let draft = FormDraft(entity: try entity(), record: Record(id: "r1", data: ["title": "牛乳", "amount": 200]))
        XCTAssertFalse(draft.hasUnsavedChanges, "読み込んだだけでは確認しない")
        draft.values["amount"] = 250
        XCTAssertTrue(draft.hasUnsavedChanges)
        draft.markClean() // 保存した
        XCTAssertFalse(draft.hasUnsavedChanges, "保存の後は確認しない")
    }

    func testReadOnlyNeverAsks() throws {
        let draft = FormDraft(entity: try entity(), record: Record(id: "r1", data: ["title": "牛乳"], author: "other"))
        draft.isReadOnly = true
        draft.values["title"] = "変更"
        XCTAssertFalse(draft.hasUnsavedChanges, "他人の記録(見るだけ)は保存できないので確認しない")
    }

    func testPrefillIsNotInput() throws {
        let draft = FormDraft(entity: try entity())
        draft.values["title"] = "カレンダーの日付から"
        draft.markClean() // `ScreenView.makeDraft` が初期値を入れた後
        XCTAssertFalse(draft.hasUnsavedChanges)
    }
}
