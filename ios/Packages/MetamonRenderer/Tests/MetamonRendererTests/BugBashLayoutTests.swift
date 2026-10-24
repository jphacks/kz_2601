import XCTest
import MetamonDSL
@testable import MetamonRenderer

/// バグバッシュ(#73)の回帰: 新規フォームで削除ボタンを描かない。
final class BugBashLayoutTests: XCTestCase {
    private func decode<T: Decodable>(_ json: String) throws -> T {
        try JSONDecoder().decode(T.self, from: Data(json.utf8))
    }

    private var expense: Entity {
        get throws { try decode(#"{"id":"expense","name":"支出","fields":[{"id":"amount","name":"金額","type":"number"}]}"#) }
    }

    // MARK: #73

    @MainActor func testDeleteButtonHiddenOnNewForm() throws {
        let delete: Action = try decode(#"{"type":"deleteRecord","entity":"expense","confirmMessage":"消す?"}"#)
        let newForm = FormDraft(entity: try expense)
        XCTAssertFalse(newForm.isEditing)
        XCTAssertFalse(ButtonLeaf.isVisible(action: delete, isEditing: newForm.isEditing), "新規フォームでは deleteRecord を描かない")
    }

    @MainActor func testDeleteButtonShownWhenEditing() throws {
        let delete: Action = try decode(#"{"type":"deleteRecord","entity":"expense"}"#)
        let editing = FormDraft(entity: try expense, record: Record(id: "r1", data: ["amount": .number(480)]))
        XCTAssertTrue(editing.isEditing)
        XCTAssertTrue(ButtonLeaf.isVisible(action: delete, isEditing: editing.isEditing), "編集モード(recordId あり)では描く")
    }

    func testOtherButtonsAlwaysVisible() throws {
        let save: Action = try decode(#"{"type":"saveRecord","entity":"expense"}"#)
        XCTAssertTrue(ButtonLeaf.isVisible(action: save, isEditing: false))
        XCTAssertTrue(ButtonLeaf.isVisible(action: nil, isEditing: false), "action 無しのボタンも従来どおり描く")
    }
}
