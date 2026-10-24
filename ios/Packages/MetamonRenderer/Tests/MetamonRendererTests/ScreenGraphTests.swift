import XCTest
import MetamonDSL
@testable import MetamonRenderer

/// #116: 画面マップの元になる画面グラフ(nav.go と同じ辺・根・到達判定 + BFS の段)。
final class ScreenGraphTests: XCTestCase {
    private func example(_ name: String) throws -> AppDSL {
        var url = URL(fileURLWithPath: #filePath)
        for _ in 0..<6 { url.deleteLastPathComponent() }
        return try AppDSL.load(contentsOf: url.appending(path: "docs/dsl/examples/\(name).json"))
    }

    private func dsl(_ json: String) throws -> AppDSL {
        try JSONDecoder().decode(AppDSL.self, from: Data(json.utf8))
    }

    private func pairs(_ g: ScreenGraph) -> [String] { g.links.map { "\($0.from)->\($0.to)" } }

    func testNavigationExample() throws {
        let g = ScreenGraph(dsl: try example("navigation"))
        XCTAssertEqual(g.nodes.map(\.id), ["a", "b", "c", "other"])
        XCTAssertEqual(g.rows, [["a", "other"], ["b", "c"]], "タブの根が上段、B / C は 1 段目")
        XCTAssertEqual(pairs(g), ["a->b", "a->c", "b->c", "c->a", "c->other", "other->b"])
        XCTAssertEqual(g.links.first { $0.id == "a->c" }?.isModal, true, "シートは破線")
        XCTAssertEqual(g.links.first { $0.id == "a->b" }?.isModal, false)
        XCTAssertEqual(g.edges.first { $0.from == "a" && $0.to == "b" }?.trigger, .button("B へ"))
        XCTAssertEqual(g.node("a")?.isRoot, true)
        XCTAssertEqual(g.node("other")?.isRoot, true)
        XCTAssertEqual(g.node("c")?.depth, 1)
        XCTAssertEqual(g.node("c")?.column, 1)
        XCTAssertEqual(g.unreachable, [])
        XCTAssertEqual(g.outgoing(from: "c").map(\.to), ["a", "other"])
        XCTAssertEqual(g.incoming(to: "b").map(\.from), ["a", "other"])
    }

    func testKakeiboMergesParallelEdges() throws {
        let g = ScreenGraph(dsl: try example("kakeibo"))
        XCTAssertEqual(g.nodes.count, 3)
        XCTAssertEqual(g.rows, [["home", "stats"], ["expense_form"]])
        // ツールバー(シート)・追加ボタン(シート)・行タップ(push)の 3 辺 → 矢印 1 本
        XCTAssertEqual(g.edges.filter { $0.from == "home" && $0.to == "expense_form" }.count, 3)
        XCTAssertEqual(pairs(g), ["home->expense_form"])
        XCTAssertEqual(g.links[0].isModal, false, "push を含むので実線")
        XCTAssertTrue(g.links[0].edges.contains { $0.trigger == .itemTap })
        XCTAssertEqual(g.node("expense_form")?.isForm, true)
        XCTAssertEqual(g.unreachable, [])
    }

    func testUnreachableGoesToLastRow() throws {
        let g = ScreenGraph(dsl: try dsl("""
        {"schemaVersion":1,"app":{"name":"t"},"initialScreen":"home","entities":[],"screens":[
          {"id":"home","title":"ホーム","components":[
            {"type":"button","label":"詳細","action":{"type":"navigate","screen":"detail"}},
            {"type":"button","label":"幽霊","action":{"type":"navigate","screen":"ghost"}}]},
          {"id":"detail","title":"詳細","toolbar":[{"label":"保存","action":{"type":"saveRecord","entity":"x","screen":"done"}}]},
          {"id":"done","title":"完了"},
          {"id":"orphan","title":""},
          {"id":"island","title":"島","components":[{"type":"button","label":"孤","action":{"type":"navigate","screen":"orphan"}}]}
        ]}
        """))
        XCTAssertEqual(g.rows, [["home"], ["detail"], ["done"], ["orphan", "island"]])
        XCTAssertEqual(g.unreachable, ["orphan", "island"], "到達できない画面から出る辺では到達扱いにしない")
        XCTAssertNil(g.node("orphan")?.depth)
        XCTAssertEqual(g.node("orphan")?.title, "orphan", "タイトルが空なら id")
        XCTAssertEqual(g.edges.first { $0.from == "detail" }?.trigger, .afterSave)
        XCTAssertEqual(g.edges.count, 4, "存在しない画面への辺も edges には残す")
        XCTAssertFalse(pairs(g).contains("home->ghost"), "矢印は描かない")
        XCTAssertEqual(ScreenGraph.roots(of: try dsl("""
        {"schemaVersion":1,"app":{"name":"t","tabs":[{"screen":"a","title":"A"}]},"initialScreen":"b","screens":[{"id":"a"},{"id":"b"}]}
        """)), ["b"], "タブ 1 個は iOS ではタブバーを出さないので initialScreen が根")
    }
}
