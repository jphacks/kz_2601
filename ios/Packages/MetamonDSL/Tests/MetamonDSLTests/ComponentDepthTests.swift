import XCTest
@testable import MetamonDSL

/// 深い入れ子の DSL: `Component.maxNestingDepth` より深い children は読まない(デコードの再帰でスタックを使い切らない)。
final class ComponentDepthTests: XCTestCase {
    private func json(_ levels: Int) -> Data {
        var s = #"{"type":"text","text":"leaf"}"#
        for _ in 0..<levels { s = #"{"type":"vstack","children":["# + s + "]}" }
        return Data(s.utf8)
    }

    private func depth(_ c: Component) -> Int {
        var d = 1, node = c
        while let child = node.children.first { node = child; d += 1 }
        return d
    }

    func testShallowTreeKeepsEveryLevel() throws {
        XCTAssertEqual(depth(try JSONDecoder().decode(Component.self, from: json(1))), 2)
        let atLimit = try JSONDecoder().decode(Component.self, from: json(Component.maxNestingDepth - 1))
        XCTAssertEqual(depth(atLimit), Component.maxNestingDepth)
    }

    /// 非同期処理のスレッドと同じ 512KB のスタックでも、JSONDecoder が読める深さ(約 250 段)で落ちない。
    func testDeepTreeIsCutAtLimitOnSmallStack() {
        let data = json(240)
        let done = expectation(description: "decoded")
        nonisolated(unsafe) var got: Int?
        let t = Thread {
            got = (try? JSONDecoder().decode(Component.self, from: data)).map(self.depth)
            done.fulfill()
        }
        t.stackSize = 512 * 1024
        t.start()
        wait(for: [done], timeout: 30)
        XCTAssertEqual(got, Component.maxNestingDepth)
    }
}
