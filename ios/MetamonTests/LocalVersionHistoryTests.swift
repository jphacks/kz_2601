import XCTest
@testable import Metamon

/// 端末の版の履歴(#968・Mac の「元に戻す / やり直す」)。iOS の `AppRuntime.undo` / `redo` の端末の履歴と同じ決まり。
final class LocalVersionHistoryTests: XCTestCase {
    func testUndoThenRedoReturnsToTheVersionBeforeUndo() {
        var history = LocalVersionHistory<Int>()
        XCTAssertFalse(history.canUndo)
        history.record(previous: 1) // 1 → 2
        history.record(previous: 2) // 2 → 3
        XCTAssertEqual(history.undo(from: 3), 2)
        XCTAssertTrue(history.canRedo)
        XCTAssertEqual(history.undo(from: 2), 1)
        XCTAssertFalse(history.canUndo)
        XCTAssertNil(history.undo(from: 1))
        // 続けて戻した分は 1 つずつやり直せる(#1883)
        XCTAssertEqual(history.redo(from: 1), 2)
        XCTAssertTrue(history.canRedo)
        // やり直した後はまた戻せる
        XCTAssertEqual(history.undo(from: 2), 1)
        XCTAssertEqual(history.redo(from: 1), 2)
        XCTAssertEqual(history.redo(from: 2), 3)
        XCTAssertFalse(history.canRedo)
        XCTAssertNil(history.redo(from: 3))
    }

    func testNewVersionDropsRedo() {
        var history = LocalVersionHistory<Int>()
        history.record(previous: 1)
        XCTAssertEqual(history.undo(from: 2), 1)
        history.record(previous: 1) // 戻した後に別の版が当たった
        XCTAssertFalse(history.canRedo)
        XCTAssertTrue(history.canUndo)
    }

    func testKeepsOnlyTheLatestVersions() {
        var history = LocalVersionHistory<Int>()
        for v in 0..<15 { history.record(previous: v) }
        XCTAssertEqual(history.past, Array(5..<15))
    }

    /// Mac は「同梱の例のまま」を nil で持つ。nil に戻せて、nil からやり直せる
    func testOptionalVersionsKeepNilAsAVersion() {
        var history = LocalVersionHistory<String?>()
        history.record(previous: nil)
        XCTAssertEqual(history.undo(from: "v2"), .some(nil))
        XCTAssertEqual(history.redo(from: nil), .some("v2"))
        XCTAssertEqual(history.undo(from: "v2"), .some(nil))
    }
}
