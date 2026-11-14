import XCTest
@testable import MetamonRenderer

/// Mac の入力 leaf の判定だけの関数(#942)を、パッケージの `swift test` で確かめる(Xcode のアプリのビルドが要らない)。
/// step から小数の桁を決める・その桁で丸めた文字にする・日本語の変換中は Return で送らない。
@MainActor
final class MacNumberInputTests: XCTestCase {
    // MARK: - step から桁

    func testDecimalsOfStep() {
        XCTAssertEqual(MacNumberInput.decimals(of: 1), 0)
        XCTAssertEqual(MacNumberInput.decimals(of: 50), 0)
        XCTAssertEqual(MacNumberInput.decimals(of: 0.5), 1)
        XCTAssertEqual(MacNumberInput.decimals(of: 0.1), 1)
        XCTAssertEqual(MacNumberInput.decimals(of: 0.3), 1, "2 進数の誤差では桁を増やさない")
        XCTAssertEqual(MacNumberInput.decimals(of: 0.25), 2)
        XCTAssertEqual(MacNumberInput.decimals(of: 0.001), 3)
        XCTAssertEqual(MacNumberInput.decimals(of: 0.0000001), 6, "細かすぎる step は 6 桁で止める")
    }

    // MARK: - 桁で丸めた文字

    func testFormatWithoutDecimalsRoundsToInt() {
        XCTAssertEqual(MacNumberInput.format(3, places: 0), "3")
        XCTAssertEqual(MacNumberInput.format(2.5, places: 0), "3", "0 桁は四捨五入")
        XCTAssertEqual(MacNumberInput.format(-2.5, places: 0), "-3")
        XCTAssertEqual(MacNumberInput.format(0, places: 0), "0")
    }

    func testFormatDropsTrailingZerosAndMinusZero() {
        XCTAssertEqual(MacNumberInput.format(1.5, places: 2), "1.5", "余った 0 は出さない")
        XCTAssertEqual(MacNumberInput.format(1.25, places: 2), "1.25")
        XCTAssertEqual(MacNumberInput.format(3, places: 2), "3", "整数なら .0 を付けない")
        XCTAssertEqual(MacNumberInput.format(1.2345, places: 2), "1.23")
        XCTAssertEqual(MacNumberInput.format(-0.004, places: 2), "0", "-0 とは出さない")
        XCTAssertEqual(MacNumberInput.format(100, places: 1), "100")
    }

    func testFormatSurvivesNonFiniteValues() {
        XCTAssertEqual(MacNumberInput.format(.nan, places: 0), "0", "数でない値は 0")
        XCTAssertEqual(MacNumberInput.format(.infinity, places: 0), String(Int.max), "落ちずに端の値")
        XCTAssertEqual(MacNumberInput.format(-.infinity, places: 0), String(Int.min))
    }

    /// ↑↓ の結果が step の桁で丸まり、2 進数の誤差(0.30000000000000004)が出ないこと
    func testSteppedUsesStepDecimals() {
        XCTAssertEqual(MacNumberInput.stepped("0.2", direction: 1, min: nil, max: nil, step: 0.1, allowsDecimal: true), "0.3")
        XCTAssertEqual(MacNumberInput.stepped("1", direction: 1, min: nil, max: nil, step: 0.25, allowsDecimal: true), "1.25")
        XCTAssertEqual(MacNumberInput.stepped("1", direction: 1, min: nil, max: nil, step: 0.25, allowsDecimal: false), "1",
                       "小数を許さない欄は整数のまま")
    }

    // MARK: - 変換中の Return

    /// 未確定の文字を持つかだけを答える作り物(本物は NSTextView)
    private final class FakeEditor: MarkedTextProviding {
        var marked: Bool
        init(marked: Bool) { self.marked = marked }
        func hasMarkedText() -> Bool { marked }
    }

    func testReturnBlockedWhileComposing() {
        let editor = FakeEditor(marked: true)
        XCTAssertFalse(IMEReturnGuard.allowsReturn(editor), "変換中の Return は確定なので送らない")
        editor.marked = false
        XCTAssertTrue(IMEReturnGuard.allowsReturn(editor), "確定した後は送れる")
    }

    func testReturnAllowedWithoutEditor() {
        XCTAssertTrue(IMEReturnGuard.allowsReturn(nil), "入力欄にいなければ Return はそのまま使う")
    }
}
