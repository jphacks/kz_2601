import XCTest
import MetamonDSL
@testable import MetamonRenderer

/// バグバッシュ(#73 / #83)の回帰: 新規フォームで削除ボタンを描かない・FAB 帯(最下段)の占有行の計算。
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

    // MARK: #83

    private func areas(_ specs: String...) -> [GridArea] { specs.map { GridArea($0)! } }

    func testFabClearanceRows() {
        // 家計簿ホーム: 右下の追加ボタンは FAB(左下)と共存 → 退避なし
        XCTAssertEqual(FabClearance.obstructedRows(areas("A1:F2", "A3:F10", "D11:F12")), 0)
        // 筋トレホーム: 全幅の保存ボタンが最下段 → 2 行ぶん上へ
        XCTAssertEqual(FabClearance.obstructedRows(areas("A1:F2", "A3:F7", "A8:F10", "A11:F12")), 2)
        // 最下 1 行だけ
        XCTAssertEqual(FabClearance.obstructedRows(areas("A12:F12")), 1)
        // B 列始まりも FAB に掛かる / C 列始まりは掛からない
        XCTAssertEqual(FabClearance.obstructedRows(areas("B11:F12")), 2)
        XCTAssertEqual(FabClearance.obstructedRows(areas("C11:F12")), 0)
        // 全面 list(A1:F12)や行 10 で終わる部品は背景扱い → 退避なし
        XCTAssertEqual(FabClearance.obstructedRows(areas("A1:F12")), 0)
        XCTAssertEqual(FabClearance.obstructedRows(areas("A3:F10")), 0)
        // 複数あれば最も上から始まるもの
        XCTAssertEqual(FabClearance.obstructedRows(areas("A12:C12", "A10:F12")), 3)
        XCTAssertEqual(FabClearance.obstructedRows([]), 0)
    }
}
