import XCTest
@testable import MetamonDSL

/// 共有レコードの更新・削除の可否(#249 / #252)。appconfig の 403 と同じ規則。
final class RecordPermissionTests: XCTestCase {
    private func entity(scope: String?, edit: String? = nil) throws -> Entity {
        var json = #"{"id":"chore","name":"家事","fields":[{"id":"title","name":"家事","type":"string"}]"#
        if let scope { json += #","scope":"\#(scope)""# }
        if let edit { json += #","edit":"\#(edit)""# }
        json += "}"
        return try JSONDecoder().decode(Entity.self, from: Data(json.utf8))
    }

    private let mine = Record(id: "r1", data: [:], author: "me")
    private let theirs = Record(id: "r2", data: [:], author: "other")
    private let unsent = Record(id: "local-x", data: [:])

    func testDecodesScopeEditAndAuthor() throws {
        let e = try entity(scope: "public", edit: "members")
        XCTAssertTrue(e.isShared)
        XCTAssertTrue(e.membersCanEdit)
        let r = try JSONDecoder().decode(Record.self, from: Data(#"{"id":"r","data":{},"author":"u1","authorName":"さくら"}"#.utf8))
        XCTAssertEqual(r.author, "u1")
        XCTAssertFalse(try entity(scope: nil, edit: "members").membersCanEdit) // private では無視
        XCTAssertFalse(try entity(scope: "team").isShared)                     // 未知の scope は private 扱い
    }

    func testAuthorOnlySharedEntity() throws {
        let e = try entity(scope: "public")
        XCTAssertTrue(RecordPermission.canEdit(mine, in: e, uid: "me"))
        XCTAssertTrue(RecordPermission.canDelete(mine, in: e, uid: "me"))
        XCTAssertFalse(RecordPermission.canEdit(theirs, in: e, uid: "me"))
        XCTAssertFalse(RecordPermission.canDelete(theirs, in: e, uid: "me"))
        XCTAssertTrue(RecordPermission.canEdit(unsent, in: e, uid: "me")) // 未送信の自分の行は author がまだ無い
    }

    func testMembersEntityUpdatesButDeletesOnlyOwn() throws {
        let e = try entity(scope: "shared", edit: "members")
        XCTAssertTrue(RecordPermission.canEdit(theirs, in: e, uid: "me"))
        XCTAssertFalse(RecordPermission.canDelete(theirs, in: e, uid: "me"))
        XCTAssertTrue(RecordPermission.canDelete(mine, in: e, uid: "me"))
    }

    func testPrivateOrUnknownUserIsNotBlocked() throws {
        XCTAssertTrue(RecordPermission.canEdit(theirs, in: try entity(scope: nil), uid: "me"))
        XCTAssertTrue(RecordPermission.canDelete(theirs, in: try entity(scope: "public"), uid: nil))
        XCTAssertTrue(RecordPermission.canEdit(theirs, in: nil, uid: "me"))
    }
}
