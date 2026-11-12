import XCTest
@testable import Metamon

/// 前 / 次のアプリ(⌥⌘↑↓)は、サイドバーに見えている(検索で絞った後の)アプリの中で移る(#1282)。
@MainActor
final class MacAppStepTests: XCTestCase {
    private let visible = ["a", "c", "e"]

    func testStepsWithinVisibleApps() {
        XCTAssertEqual(MacRootView.step(from: "a", offset: 1, in: visible), "c", "隠れた b は飛ばす")
        XCTAssertEqual(MacRootView.step(from: "c", offset: -1, in: visible), "a")
        XCTAssertNil(MacRootView.step(from: "e", offset: 1, in: visible), "末尾より先は無い")
        XCTAssertNil(MacRootView.step(from: "a", offset: -1, in: visible), "先頭より前は無い")
    }

    func testHiddenOrNoSelectionStartsFromFirstVisible() {
        XCTAssertEqual(MacRootView.step(from: "b", offset: 1, in: visible), "a", "検索で隠れたアプリからは、見えている先頭へ")
        XCTAssertNil(MacRootView.step(from: "b", offset: -1, in: visible))
        XCTAssertEqual(MacRootView.step(from: nil, offset: 1, in: visible), "a")
        XCTAssertNil(MacRootView.step(from: nil, offset: 1, in: []), "見えているアプリが無ければ移らない")
    }
}
