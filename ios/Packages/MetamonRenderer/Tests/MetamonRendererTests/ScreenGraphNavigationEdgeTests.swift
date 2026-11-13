import XCTest
import MetamonDSL
@testable import MetamonRenderer

/// #2848: 画面マップの隣・道・画面を開く(#970・#1322)の端と recordId。
/// `--filter ScreenGraphNavigationTests` で一緒に回るよう、既存のクラスに足す。
extension ScreenGraphNavigationTests {
    private func navigationExample() throws -> AppDSL {
        var url = URL(fileURLWithPath: #filePath)
        for _ in 0..<6 { url.deleteLastPathComponent() }
        return try AppDSL.load(contentsOf: url.appending(path: "docs/dsl/examples/navigation.json"))
    }

    /// 段: [home] / [one] / [two]・island は根から行けない(タブなし)
    private func chainDSL() throws -> AppDSL {
        try JSONDecoder().decode(AppDSL.self, from: Data("""
        {"schemaVersion":1,"app":{"name":"t"},"initialScreen":"home","screens":[
          {"id":"home","components":[{"type":"button","label":"1","action":{"type":"navigate","screen":"one"}}]},
          {"id":"one","components":[{"type":"button","label":"2","action":{"type":"navigate","screen":"two"}}]},
          {"id":"two"},{"id":"island"}]}
        """.utf8))
    }

    // MARK: neighbor

    func testNeighborLeftRightAndEdges() throws {
        let g = ScreenGraph(dsl: try navigationExample())
        XCTAssertEqual(g.neighbor(of: "other", .left), "a")
        XCTAssertNil(g.neighbor(of: "other", .right), "段の右端")
        XCTAssertNil(g.neighbor(of: "other", .up), "上の段が無い")
        XCTAssertEqual(g.neighbor(of: "b", .right), "c")
        XCTAssertEqual(g.neighbor(of: "c", .left), "b")
        XCTAssertNil(g.neighbor(of: "b", .left), "段の左端")
        XCTAssertEqual(g.neighbor(of: "b", .up), "a")
    }

    func testNeighborFromNilOrUnknownIsFirstCard() throws {
        let g = ScreenGraph(dsl: try navigationExample())
        for move: ScreenGraph.Move in [.up, .down, .left, .right] {
            XCTAssertEqual(g.neighbor(of: nil, move), "a")
            XCTAssertEqual(g.neighbor(of: "nope", move), "a", "無い画面からは先頭")
        }
    }

    func testNeighborInSingleColumnChain() throws {
        let g = ScreenGraph(dsl: try chainDSL())
        XCTAssertEqual(g.neighbor(of: "home", .down), "one")
        XCTAssertEqual(g.neighbor(of: "two", .up), "one")
        XCTAssertNil(g.neighbor(of: "one", .left))
        XCTAssertNil(g.neighbor(of: "one", .right))
    }

    // MARK: path

    func testPathReachableAndUnreachable() throws {
        let g = ScreenGraph(dsl: try chainDSL())
        XCTAssertEqual(g.path(to: "home")?.map(\.id), [], "根は空の道")
        XCTAssertEqual(g.path(to: "two")?.map(\.to), ["one", "two"], "根から順の矢印")
        XCTAssertEqual(g.path(to: "two")?.first?.from, "home")
        XCTAssertNil(g.path(to: "island"), "根から行けない画面は nil")
        XCTAssertNil(g.path(to: "nope"), "無い画面は nil")
    }

    func testPathToOtherTabRootIsEmpty() throws {
        let g = ScreenGraph(dsl: try navigationExample())
        XCTAssertEqual(g.path(to: "other")?.map(\.id), [], "タブはどれも根")
    }

    // MARK: reveal

    func testRevealWithRecordIdSetsOnlyLastScreen() throws {
        let d = try chainDSL()
        let s = try XCTUnwrap(NavigationResolver.reveal("two", recordId: "r1", graph: ScreenGraph(dsl: d), dsl: d, from: NavigationState()))
        XCTAssertEqual(s.paths["main"]?.map(\.screen), ["one", "two"])
        XCTAssertEqual(s.paths["main"]?.map(\.recordId), [nil, "r1"], "記録は最後の画面だけ")
        XCTAssertEqual(NavigationResolver.top(s, dsl: d), Route(screen: "two", recordId: "r1"))
    }

    func testRevealInsideTabWithRecordId() throws {
        let d = try navigationExample()
        let s = try XCTUnwrap(NavigationResolver.reveal("b", recordId: "r2", graph: ScreenGraph(dsl: d), dsl: d,
                                                        from: NavigationState(selectedTab: "other")))
        XCTAssertEqual(s.selectedTab, "a", "道の始まりのタブへ切り替える")
        XCTAssertEqual(s.paths["tab:a"], [Route(screen: "b", recordId: "r2", stack: "tab:a")])
        XCTAssertEqual(NavigationResolver.top(s, dsl: d).recordId, "r2")
    }

    func testRevealUnreachableWithRecordIdStacksOnRoot() throws {
        let d = try chainDSL()
        let s = try XCTUnwrap(NavigationResolver.reveal("island", recordId: "r3", graph: ScreenGraph(dsl: d), dsl: d,
                                                        from: NavigationState()))
        XCTAssertEqual(s.paths["main"], [Route(screen: "island", recordId: "r3")])
    }

    func testRevealRootClearsStack() throws {
        let d = try chainDSL()
        let g = ScreenGraph(dsl: d)
        let deep = try XCTUnwrap(NavigationResolver.reveal("two", graph: g, dsl: d, from: NavigationState()))
        let root = try XCTUnwrap(NavigationResolver.reveal("home", graph: g, dsl: d, from: deep))
        XCTAssertEqual(root.paths["main"], [], "根を開くと積んだ画面は外す")
        XCTAssertEqual(NavigationResolver.top(root, dsl: d).screen, "home")
    }

    func testRevealUnknownScreenIsNil() throws {
        let d = try chainDSL()
        XCTAssertNil(NavigationResolver.reveal("nope", recordId: "r", graph: ScreenGraph(dsl: d), dsl: d, from: NavigationState()))
    }
}
