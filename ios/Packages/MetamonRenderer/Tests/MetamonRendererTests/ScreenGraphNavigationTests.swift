import XCTest
import MetamonDSL
@testable import MetamonRenderer

/// #970: 画面マップの矢印キーでの移動・マップから画面を開く(Mac)。
final class ScreenGraphNavigationTests: XCTestCase {
    private func example(_ name: String) throws -> AppDSL {
        var url = URL(fileURLWithPath: #filePath)
        for _ in 0..<6 { url.deleteLastPathComponent() }
        return try AppDSL.load(contentsOf: url.appending(path: "docs/dsl/examples/\(name).json"))
    }

    private func dsl(_ json: String) throws -> AppDSL {
        try JSONDecoder().decode(AppDSL.self, from: Data(json.utf8))
    }

    /// 段: [a, other] / [b, c]
    func testArrowKeysMoveBetweenCards() throws {
        let g = ScreenGraph(dsl: try example("navigation"))
        XCTAssertEqual(g.readingOrder, ["a", "other", "b", "c"])
        XCTAssertEqual(g.neighbor(of: nil, .down), "a", "未選択なら先頭")
        XCTAssertEqual(g.neighbor(of: "a", .right), "other")
        XCTAssertNil(g.neighbor(of: "a", .left), "端では動かない")
        XCTAssertEqual(g.neighbor(of: "a", .down), "b")
        XCTAssertEqual(g.neighbor(of: "other", .down), "c")
        XCTAssertEqual(g.neighbor(of: "c", .up), "other")
        XCTAssertNil(g.neighbor(of: "c", .down))
        XCTAssertNil(g.neighbor(of: "a", .up))
    }

    func testUpDownPicksNearestCenteredCard() throws {
        // 段: [home] / [x, y, z]。中央揃えなので home の真下は y
        let g = ScreenGraph(dsl: try dsl("""
        {"schemaVersion":1,"app":{"name":"t"},"initialScreen":"home","screens":[
          {"id":"home","components":[
            {"type":"button","label":"x","action":{"type":"navigate","screen":"x"}},
            {"type":"button","label":"y","action":{"type":"navigate","screen":"y"}},
            {"type":"button","label":"z","action":{"type":"navigate","screen":"z"}}]},
          {"id":"x"},{"id":"y"},{"id":"z"}]}
        """))
        XCTAssertEqual(g.rows, [["home"], ["x", "y", "z"]])
        XCTAssertEqual(g.neighbor(of: "home", .down), "y")
        XCTAssertEqual(g.neighbor(of: "z", .up), "home")
    }

    func testPathFromRoot() throws {
        let g = ScreenGraph(dsl: try example("navigation"))
        XCTAssertEqual(g.path(to: "a")?.map(\.id), [])
        XCTAssertEqual(g.path(to: "c")?.map(\.id), ["a->c"], "最短の道")
        XCTAssertEqual(g.path(to: "b")?.map(\.id), ["a->b"])
        XCTAssertNil(g.path(to: "nope"))
    }

    func testRevealPushesOnTabStack() throws {
        let d = try example("navigation")
        let g = ScreenGraph(dsl: d)
        let open = NavigationState(paths: ["tab:other": [Route(screen: "b", stack: "tab:other")]], selectedTab: "other",
                                   sheet: Route(screen: "c", stack: "sheet"))
        let s = try XCTUnwrap(NavigationResolver.reveal("b", graph: g, dsl: d, from: open))
        XCTAssertEqual(s.selectedTab, "a")
        XCTAssertEqual(s.paths["tab:a"], [Route(screen: "b", stack: "tab:a")])
        XCTAssertNil(s.sheet, "開いていたシートは閉じる")
        XCTAssertEqual(NavigationResolver.top(s, dsl: d).screen, "b")
    }

    func testRevealOpensSheetAndTabRoot() throws {
        let d = try example("navigation")
        let g = ScreenGraph(dsl: d)
        let sheet = try XCTUnwrap(NavigationResolver.reveal("c", graph: g, dsl: d, from: NavigationState(selectedTab: "a")))
        XCTAssertEqual(sheet.sheet, Route(screen: "c", stack: "sheet"), "a->c はシート")
        XCTAssertEqual(NavigationResolver.top(sheet, dsl: d).screen, "c")

        let tab = try XCTUnwrap(NavigationResolver.reveal("other", graph: g, dsl: d, from: sheet))
        XCTAssertEqual(tab.selectedTab, "other")
        XCTAssertEqual(tab.paths["tab:other"], [])
        XCTAssertNil(tab.sheet)
        XCTAssertNil(NavigationResolver.reveal("nope", graph: g, dsl: d, from: tab))
    }

    func testRevealUnreachableAndChain() throws {
        let d = try dsl("""
        {"schemaVersion":1,"app":{"name":"t"},"initialScreen":"home","screens":[
          {"id":"home","components":[{"type":"button","label":"1","action":{"type":"navigate","screen":"one"}}]},
          {"id":"one","components":[{"type":"button","label":"2","action":{"type":"navigate","screen":"two"}}]},
          {"id":"two"},{"id":"island"}]}
        """)
        let g = ScreenGraph(dsl: d)
        let chain = try XCTUnwrap(NavigationResolver.reveal("two", graph: g, dsl: d, from: NavigationState()))
        XCTAssertEqual(chain.paths["main"]?.map(\.screen), ["one", "two"], "根から順に積むので戻るで 1 つずつ戻れる")
        let island = try XCTUnwrap(NavigationResolver.reveal("island", graph: g, dsl: d, from: chain))
        XCTAssertEqual(island.paths["main"], [Route(screen: "island")], "行けない画面は根の上に 1 枚")
    }
}
