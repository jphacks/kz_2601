import XCTest
import MetamonDSL
@testable import MetamonRenderer

/// #416: 右上の＋(toolbar)と button が同じフォームを開くなら、ナビバーには出さない(button だけ残す)。
final class DuplicateAddControlTests: XCTestCase {
    private func dsl(toolbar: String, components: String) throws -> AppDSL {
        let json = """
        {"schemaVersion":1,"app":{"name":"t"},"initialScreen":"home",
         "entities":[{"id":"log","name":"記録","fields":[{"id":"title","name":"内容","type":"string"}]}],
         "screens":[
          {"id":"home","title":"ホーム","toolbar":[\(toolbar)],"components":[\(components)]},
          {"id":"settings","title":"設定","components":[]},
          {"id":"form","title":"追加","entity":"log","components":[]}]}
        """
        return try JSONDecoder().decode(AppDSL.self, from: Data(json.utf8))
    }

    private func labels(_ d: AppDSL) throws -> [String] {
        DuplicateAddControl.toolbar(of: try XCTUnwrap(d.screen("home")), in: d).map(\.label)
    }

    private let plus = #"{"label":"追加","icon":"plus","action":{"type":"navigate","screen":"form","presentation":"sheet"}}"#
    private let addButton = #"{"type":"button","label":"記録を追加","area":"D11:F12","action":{"type":"navigate","screen":"form","presentation":"sheet"}}"#

    func testHidesToolbarPlusWhenButtonOpensSameForm() throws {
        XCTAssertEqual(try labels(dsl(toolbar: plus, components: addButton)), [])
        // コンテナの中の button も数える
        let inSection = #"{"type":"section","title":"s","children":["# + addButton + "]}"
        XCTAssertEqual(try labels(dsl(toolbar: plus, components: inSection)), [])
    }

    func testKeepsToolbarOtherwise() throws {
        XCTAssertEqual(try labels(dsl(toolbar: plus, components: "")), ["追加"], "button が無ければ右上の＋を残す")
        let settings = #"{"label":"設定","action":{"type":"navigate","screen":"settings"}}"#
        let settingsButton = #"{"type":"button","label":"設定","action":{"type":"navigate","screen":"settings"}}"#
        XCTAssertEqual(try labels(dsl(toolbar: settings, components: settingsButton)), ["設定"], "フォーム以外への移動は対象外")
        let list = #"{"type":"list","entity":"log","item":{"title":"title"},"onItemTap":{"type":"navigate","screen":"form"}}"#
        XCTAssertEqual(try labels(dsl(toolbar: plus, components: list)), ["追加"], "行タップ(編集)は追加ではない")
    }

    /// 実際に生成された保存済みの DSL(お小遣い帳: 右上の＋ と 右下の「記録を追加」)。
    func testSavedDocument() throws {
        var url = URL(fileURLWithPath: #filePath)
        for _ in 0..<6 { url.deleteLastPathComponent() }
        let d = try AppDSL.load(contentsOf: url.appending(path: "backend/internal/ai/examples/okozukai.json"))
        let home = try XCTUnwrap(d.screen("home"))
        XCTAssertEqual(home.toolbar.count, 1)
        XCTAssertEqual(DuplicateAddControl.toolbar(of: home, in: d).count, 0)
        XCTAssertEqual(home.allComponents.filter { $0.type == .button && $0.action?.screen == "income_form" }.count, 1, "右下の追加ボタンは残る")
        // 画面マップも、出ていない＋を入口に数えない
        let edges = ScreenGraph.edges(of: d).filter { $0.from == "home" && $0.to == "income_form" }
        XCTAssertFalse(edges.contains { if case .toolbar = $0.trigger { true } else { false } })
    }
}
