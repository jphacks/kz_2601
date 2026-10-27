import XCTest
@testable import MetamonDSL

/// #253: entity.scope を読む(画面の「誰でも見られます」表示用)。未知・省略は private。
final class EntityScopeTests: XCTestCase {
    private func dsl(_ entities: String) throws -> AppDSL {
        let json = """
        {"schemaVersion":1,"app":{"name":"t"},"initialScreen":"home","entities":[\(entities)],
         "screens":[{"id":"home","title":"t","components":[]}]}
        """
        return try JSONDecoder().decode(AppDSL.self, from: Data(json.utf8))
    }

    func testReadsScopePerEntity() throws {
        let d = try dsl("""
        {"id":"r","name":"予約","scope":"public","fields":[{"id":"n","name":"名前","type":"string"}]},
        {"id":"m","name":"メンバー","scope":"shared","fields":[]},
        {"id":"p","name":"日記","fields":[]},
        {"id":"x","name":"謎","scope":"team","fields":[]}
        """)
        XCTAssertEqual(d.scope(of: "r"), .public)
        XCTAssertEqual(d.scope(of: "m"), .shared)
        XCTAssertEqual(d.scope(of: "p"), .private)
        XCTAssertEqual(d.scope(of: "x"), .private, "未知の scope は private 扱い")
        XCTAssertEqual(d.scope(of: "missing"), .private)
        XCTAssertTrue(d.scope(of: "r").isVisibleToOthers)
        XCTAssertTrue(d.scope(of: "m").isVisibleToOthers)
        XCTAssertFalse(d.scope(of: "p").isVisibleToOthers)
        XCTAssertEqual(d.entities.count, 4, "scope は Entity のデコードを邪魔しない")
    }

    func testBrokenEntityDoesNotBreakScopes() throws {
        let d = try dsl(#"{"name":"id が無い"}, {"id":"r","name":"予約","scope":"public","fields":[]}"#)
        XCTAssertEqual(d.scope(of: "r"), .public)
    }

    func testBundledExamplesArePrivate() throws {
        let kakeibo = try ExamplesFixture.load("kakeibo")
        XCTAssertTrue(kakeibo.entities.allSatisfy { kakeibo.scope(of: $0.id) == .private })
    }
}
