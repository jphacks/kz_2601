import XCTest
import MetamonDSL
@testable import MetamonRenderer

/// #108 list.item.toggle: 行頭のチェック丸が出る条件・楽観的更新の状態遷移・schema との一致。
final class ListToggleTests: XCTestCase {
    private static let repoRoot: URL = {
        var url = URL(fileURLWithPath: #filePath)
        for _ in 0..<6 { url.deleteLastPathComponent() }
        return url
    }()

    private func shopping() throws -> AppDSL {
        try AppDSL.load(contentsOf: Self.repoRoot.appending(path: "docs/dsl/examples/shopping.json"))
    }

    private func homeLists(_ dsl: AppDSL) -> [Component] {
        dsl.screen("home")?.components.flatMap { $0.children ?? [] }.filter { $0.type == .list } ?? []
    }

    func testShoppingRowsShowToggle() throws {
        let dsl = try shopping()
        let entity = dsl.entity("groceries")
        let lists = homeLists(dsl)
        XCTAssertEqual(lists.count, 2)
        for list in lists {
            XCTAssertEqual(ListToggle.field(list.item, in: entity)?.id, "done", "買うもの/買ったもの の両方に丸が出る")
        }
        // 他の正典サンプルの list には丸が出ない(既存の見た目を変えない)
        let kakeibo = try AppDSL.load(contentsOf: Self.repoRoot.appending(path: "docs/dsl/examples/kakeibo.json"))
        let list = try XCTUnwrap(kakeibo.screen("home")?.components.flatMap { $0.children ?? [] }.first { $0.type == .list })
        XCTAssertNil(ListToggle.field(list.item, in: kakeibo.entity("expense")))
    }

    func testToggleNeedsStoredBoolean() throws {
        let entity = try JSONDecoder().decode(Entity.self, from: Data(#"""
        {"id":"t","name":"T","fields":[{"id":"name","name":"n","type":"string"},{"id":"done","name":"d","type":"boolean"},
          {"id":"auto","name":"a","type":"boolean","computed":{"op":"eq","args":[{"ref":"name"},"x"]}}]}
        """#.utf8))
        XCTAssertEqual(ListToggle.field(ListItem(title: "name", toggle: "done"), in: entity)?.id, "done")
        XCTAssertNil(ListToggle.field(ListItem(title: "name", toggle: "name"), in: entity), "boolean 以外")
        XCTAssertNil(ListToggle.field(ListItem(title: "name", toggle: "auto"), in: entity), "computed は保存できない")
        XCTAssertNil(ListToggle.field(ListItem(title: "name", toggle: "ghost"), in: entity), "存在しない field")
        XCTAssertNil(ListToggle.field(ListItem(title: "name"), in: entity), "toggle 無し")
    }

    func testIsOnReadsStoredAndPending() {
        let off = Record(id: "1", data: ["done": false])
        let on = Record(id: "2", data: ["done": true])
        XCTAssertFalse(ListToggle.isOn(off, field: "done", overrides: [:]))
        XCTAssertTrue(ListToggle.isOn(on, field: "done", overrides: [:]))
        XCTAssertTrue(ListToggle.isOn(off, field: "done", overrides: ["1": true]), "保存待ちの値を優先(楽観的)")
        XCTAssertFalse(ListToggle.isOn(Record(id: "3", data: [:]), field: "done", overrides: [:]), "未設定は未チェック")
        XCTAssertTrue(ListToggle.isOn(Record(id: "4", data: ["done": "true"]), field: "done", overrides: [:]))
    }

    func testSettleRevertsOrKeepsLatestTap() {
        // 1 回タップ → 保存結果(成功/失敗どちらでも)で保存待ちを外す = ストアの値を見る
        var overrides = ["1": true]
        ListToggle.settle(&overrides, recordId: "1", requested: true)
        XCTAssertNil(overrides["1"])
        // 失敗時: ストアは false のまま → 表示は元に戻る
        XCTAssertFalse(ListToggle.isOn(Record(id: "1", data: ["done": false]), field: "done", overrides: overrides))
        // 連打: 1 回目(true)の結果が返る前に 2 回目(false)を押したら、2 回目の表示を残す
        overrides = ["1": false]
        ListToggle.settle(&overrides, recordId: "1", requested: true)
        XCTAssertEqual(overrides["1"], false)
    }

    @MainActor func testUnboundRuntimeDoesNotPretendToSave() async {
        let saved = await UnboundDSLRuntime.shared.updateRecord(entity: "groceries", recordId: "g1", data: ["done": true])
        XCTAssertFalse(saved, "保存できないランタイムは false を返し、表示は元に戻る")
    }

    /// schema の ListItem のキーと Swift の ListItem のプロパティが一致(toggle を足し忘れると赤)。
    func testListItemKeysMatchSchema() throws {
        let data = try Data(contentsOf: Self.repoRoot.appending(path: "docs/dsl/dsl.schema.json"))
        let root = try XCTUnwrap(try JSONSerialization.jsonObject(with: data) as? [String: Any])
        let item = try XCTUnwrap((root["$defs"] as? [String: Any])?["ListItem"] as? [String: Any])
        let keys = Set(try XCTUnwrap(item["properties"] as? [String: Any]).keys)
        let props = Set(Mirror(reflecting: ListItem(title: "t")).children.compactMap(\.label))
        XCTAssertEqual(keys, props)
    }
}
