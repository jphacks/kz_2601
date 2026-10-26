import XCTest
import MetamonDSL
@testable import MetamonRenderer

/// #249 / #252: 他人の共有レコードでは保存・削除を出さず、理由を出す。
final class SharedRecordPermissionTests: XCTestCase {
    private func decode<T: Decodable>(_ json: String) throws -> T {
        try JSONDecoder().decode(T.self, from: Data(json.utf8))
    }

    private var save: Action { get throws { try decode(#"{"type":"saveRecord","entity":"chore"}"#) } }
    private var delete: Action { get throws { try decode(#"{"type":"deleteRecord","entity":"chore"}"#) } }
    private var navigate: Action { get throws { try decode(#"{"type":"navigate","screen":"home"}"#) } }

    func testButtonsHiddenForReadOnlyRecord() throws {
        XCTAssertFalse(ButtonLeaf.isVisible(action: try save, isEditing: true, readOnly: true), "更新できないレコードでは保存を出さない")
        XCTAssertTrue(ButtonLeaf.isVisible(action: try save, isEditing: true, readOnly: false))
        XCTAssertFalse(ButtonLeaf.isVisible(action: try delete, isEditing: true, canDelete: false), "作った人でなければ削除を出さない")
        XCTAssertTrue(ButtonLeaf.isVisible(action: try delete, isEditing: true, readOnly: false, canDelete: true))
        XCTAssertTrue(ButtonLeaf.isVisible(action: try navigate, isEditing: true, readOnly: true, canDelete: false), "移動ボタンは残す")
    }

    /// edit:"members" の家事: 他人のレコードでも保存は出す・削除は出さない。
    @MainActor func testFormFlagsDriveButtons() throws {
        let chore: Entity = try decode(#"{"id":"chore","name":"家事","scope":"public","edit":"members","fields":[{"id":"done","name":"済み","type":"boolean"}]}"#)
        let theirs = Record(id: "r1", data: ["done": .bool(false)], author: "other")
        let form = FormDraft(entity: chore, record: theirs)
        form.isReadOnly = !RecordPermission.canEdit(theirs, in: chore, uid: "me")
        form.canDelete = RecordPermission.canDelete(theirs, in: chore, uid: "me")
        XCTAssertTrue(ButtonLeaf.isVisible(action: try save, form: form))
        XCTAssertFalse(ButtonLeaf.isVisible(action: try delete, form: form))
    }

    func testReadOnlyNoticeOnlyOnEditForms() throws {
        let form: Screen = try decode(#"{"id":"edit","title":"編集","entity":"chore","toolbar":[{"label":"保存","action":{"type":"saveRecord","entity":"chore"}}],"components":[]}"#)
        let inline: Screen = try decode(#"{"id":"edit2","title":"編集","entity":"chore","components":[{"type":"vstack","children":[{"type":"button","label":"保存","action":{"type":"saveRecord","entity":"chore"}}]}]}"#)
        let detail: Screen = try decode(#"{"id":"detail","title":"詳細","entity":"chore","components":[{"type":"text","text":"詳細"}]}"#)
        XCTAssertTrue(FormSaveBar.hasSave(in: form))
        XCTAssertTrue(FormSaveBar.hasSave(in: inline))
        XCTAssertFalse(FormSaveBar.hasSave(in: detail), "表示だけの画面には理由を出さない")
    }
}
