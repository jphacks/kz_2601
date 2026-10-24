import XCTest
@testable import MetamonRenderer

/// #143: キーボード表示中の FAB / グリッドの行高さ。
final class KeyboardSupportTests: XCTestCase {
    func testFabHiddenWhileKeyboardVisible() {
        XCTAssertTrue(FabVisibility.isShown(floating: true, formVisible: false, keyboardVisible: false))
        XCTAssertFalse(FabVisibility.isShown(floating: true, formVisible: false, keyboardVisible: true))
        // 従来どおり: フォーム画面 / floating 以外は出さない
        XCTAssertFalse(FabVisibility.isShown(floating: true, formVisible: true, keyboardVisible: false))
        XCTAssertFalse(FabVisibility.isShown(floating: false, formVisible: false, keyboardVisible: false))
    }

    func testHardwareKeyboardBarIsNotKeyboard() {
        XCTAssertFalse(KeyboardState.counts(height: 0))
        XCTAssertFalse(KeyboardState.counts(height: 69)) // ハードウェアキーボード接続時の候補バー
        XCTAssertTrue(KeyboardState.counts(height: 336))
    }

    func testGridKeepsRowHeightWhenKeyboardShrinksViewport() {
        // 未計測(0)なら今の高さ
        XCTAssertEqual(GridLayoutHeight.resolve(current: 700, stable: 0), 700)
        // キーボードで 700 → 400 に縮んでも行の高さはキーボード前の 700 基準(セルを潰さずスクロールに逃がす)
        XCTAssertEqual(GridLayoutHeight.resolve(current: 400, stable: 700), 700)
        // 回転等で広がった時は今の高さ
        XCTAssertEqual(GridLayoutHeight.resolve(current: 800, stable: 700), 800)
    }
}
