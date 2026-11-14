import XCTest
import SwiftUI
import MetamonDSL
@testable import MetamonRenderer

/// 深い入れ子の DSL: `Component.maxNestingDepth` 段より深い所は読まず・描かない(スタックを使い切らない)。
@MainActor
final class DSLViewDepthTests: XCTestCase {
    private final class Seen: @unchecked Sendable { var leaf = false }

    private func nested(_ levels: Int) throws -> Component {
        var json = #"{"type":"text","text":"leaf"}"#
        for _ in 0..<levels { json = #"{"type":"vstack","children":["# + json + "]}" }
        return try JSONDecoder().decode(Component.self, from: Data(json.utf8))
    }

    /// 葉まで描いたかを返す(葉の text の builder が呼ばれたか)。`outer` = 外側にある DSLView の数。
    private func rendersLeaf(_ c: Component, outer: Int = 0) -> Bool {
        let seen = Seen()
        let registry = ComponentRegistry.standard.registering(.text) { _, _ in
            let _ = { seen.leaf = true }()
            Text("leaf")
        }
        let view = DSLView(component: c, ctx: RenderContext(route: Route(screen: "home")))
            .environment(\.componentRegistry, registry)
            .environment(\.dslNestingDepth, outer)
            .frame(width: 390, height: 600)
        _ = ImageRenderer(content: view).platformImage
        return seen.leaf
    }

    func testRendersUpToLimit() throws {
        XCTAssertTrue(rendersLeaf(try nested(1)))
        XCTAssertTrue(rendersLeaf(try nested(DSLView.maxNestingDepth - 1)))
        XCTAssertTrue(rendersLeaf(try nested(1), outer: DSLView.maxNestingDepth - 2))
    }

    func testStopsDrawingPastLimit() throws {
        XCTAssertFalse(rendersLeaf(try nested(1), outer: DSLView.maxNestingDepth - 1))
        XCTAssertFalse(rendersLeaf(try nested(DSLView.maxNestingDepth)))
    }

    /// 深い DSL(JSONDecoder が読める約 250 段まで)でも読むところで落ちない。上限より深い children は空。
    func testDeepTreeDecodesAndRenders() throws {
        let c = try nested(240)
        var depth = 1, node = c
        while let child = node.children.first { node = child; depth += 1 }
        XCTAssertEqual(depth, Component.maxNestingDepth)
        XCTAssertFalse(rendersLeaf(c))
    }
}
