import XCTest
import MetamonDSL
@testable import MetamonRenderer

/// M10: 保存で弾かれた空の必須欄の印と、スクロール先。
final class RequiredFieldMarkTests: XCTestCase {
    private func entity() throws -> Entity {
        try JSONDecoder().decode(Entity.self, from: Data(#"""
        {"id":"memo","name":"メモ","fields":[
          {"id":"title","name":"題名","type":"string","required":true},
          {"id":"amount","name":"金額","type":"number","required":true},
          {"id":"secret","name":"正解","type":"string","required":true,"hidden":true},
          {"id":"note","name":"メモ","type":"string"}]}
        """#.utf8))
    }

    private func screen(_ components: String) throws -> Screen {
        try JSONDecoder().decode(Screen.self, from: Data(#"{"id":"form","title":"入力","entity":"memo","components":\#(components)}"#.utf8))
    }

    @MainActor func testFlagReturnsMissingNamesAndCounts() throws {
        let draft = FormDraft(entity: try entity())
        XCTAssertEqual(draft.flagCount, 0)
        XCTAssertEqual(draft.flagMissingRequired(), ["題名", "金額", "正解"])
        XCTAssertEqual(draft.flaggedFields, ["title", "amount", "secret"])
        XCTAssertEqual(draft.flagCount, 1)
        draft.flagMissingRequired()
        XCTAssertEqual(draft.flagCount, 2, "同じ欄でも押すたびに数える(スクロールし直す)")
    }

    @MainActor func testNothingMissingIsNotARejection() throws {
        let draft = FormDraft(entity: try entity())
        draft.values = ["title": "牛乳", "amount": 200, "secret": "A"]
        XCTAssertEqual(draft.flagMissingRequired(), [])
        XCTAssertEqual(draft.flagCount, 0)
        XCTAssertTrue(draft.flaggedFields.isEmpty)
    }

    @MainActor func testMarkIsNotShownBeforeSaving() throws {
        XCTAssertFalse(FormDraft(entity: try entity()).isFlagged("title"), "開いただけでは赤くしない")
    }

    @MainActor func testMarkClearsWhenFilledAndReturnsWhenEmptiedAgain() throws {
        let draft = FormDraft(entity: try entity())
        draft.flagMissingRequired()
        XCTAssertTrue(draft.isFlagged("title"))
        draft.values["title"] = "牛乳"
        XCTAssertFalse(draft.isFlagged("title"))
        XCTAssertTrue(draft.isFlagged("amount"), "他の欄の印は残る")
        draft.values["title"] = ""
        XCTAssertTrue(draft.isFlagged("title"))
    }

    @MainActor func testOptionalFieldIsNeverFlagged() throws {
        let draft = FormDraft(entity: try entity())
        draft.flagMissingRequired()
        XCTAssertFalse(draft.isFlagged("note"))
    }

    @MainActor func testResetAndLoadClearMarks() throws {
        let draft = FormDraft(entity: try entity())
        draft.flagMissingRequired()
        draft.reset()
        XCTAssertFalse(draft.isFlagged("title"))
        draft.flagMissingRequired()
        draft.load(Record(id: "r1", data: [:]))
        XCTAssertFalse(draft.isFlagged("title"))
    }

    /// 表示用の部品(`field` が集計の対象)には印を付けない。
    @MainActor func testOnlyInputsAreMarked() throws {
        let draft = FormDraft(entity: try entity())
        let s = try screen(#"""
        [{"id":"s","type":"stat","field":"amount"},
         {"id":"o","type":"textField","entity":"other","field":"title"},
         {"id":"p","type":"photoField","field":"title"},
         {"id":"t","type":"textField","field":"title"}]
        """#)
        let byId = Dictionary(uniqueKeysWithValues: s.components.map { ($0.id, $0) })
        XCTAssertNil(RequiredFieldMark.boundField(of: try XCTUnwrap(byId["s"]), in: draft))
        XCTAssertNil(RequiredFieldMark.boundField(of: try XCTUnwrap(byId["o"]), in: draft))
        XCTAssertEqual(RequiredFieldMark.boundField(of: try XCTUnwrap(byId["p"]), in: draft), "title")
        XCTAssertEqual(RequiredFieldMark.boundField(of: try XCTUnwrap(byId["t"]), in: draft), "title")
    }

    /// entity の並びではなく、画面の並びで最初の欄へ。
    @MainActor func testScrollsToFirstOnScreenOrder() throws {
        let draft = FormDraft(entity: try entity())
        draft.flagMissingRequired()
        let s = try screen(#"""
        [{"id":"b","type":"textField","field":"title","area":"A3:F3"},
         {"id":"a","type":"numberField","field":"amount","area":"A1:F2"}]
        """#)
        XCTAssertEqual(RequiredFieldScroller.firstFlagged(in: s, form: draft, currentUid: nil), "amount")
    }

    @MainActor func testScrollFindsFieldInsideContainer() throws {
        let draft = FormDraft(entity: try entity())
        draft.flagMissingRequired()
        let s = try screen(#"""
        [{"id":"sec","type":"section","title":"基本","children":[{"id":"t","type":"textField","field":"title"}]}]
        """#)
        XCTAssertEqual(RequiredFieldScroller.firstFlagged(in: s, form: draft, currentUid: nil), "title")
    }

    /// 入力部品が画面に無い field(トーストだけが頼り)と、回答者に隠す field は飛ばす。
    @MainActor func testScrollSkipsFieldsNotOnScreenAndHidden() throws {
        let draft = FormDraft(entity: try entity(), record: Record(id: "r1", data: [:], author: "author"))
        draft.flagMissingRequired()
        let s = try screen(#"""
        [{"id":"h","type":"textField","field":"secret","area":"A1:F1"},
         {"id":"a","type":"numberField","field":"amount","area":"A2:F2"}]
        """#)
        XCTAssertEqual(RequiredFieldScroller.firstFlagged(in: s, form: draft, currentUid: "someone"), "amount")
        XCTAssertEqual(RequiredFieldScroller.firstFlagged(in: s, form: draft, currentUid: "author"), "secret")
        draft.values["amount"] = 1
        XCTAssertNil(RequiredFieldScroller.firstFlagged(in: s, form: draft, currentUid: "someone"))
    }
}
