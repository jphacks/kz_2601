import XCTest
import MetamonServices
@testable import Metamon

/// #2243: ショートカット・Siri の名前検索(`IntentMatching.indices`)。
/// 「家計簿」と言った時にどのアプリ・記録先を候補に出すかを固定する。
final class IntentMatchingTests: XCTestCase {
    private func hits(_ query: String, _ names: [String]) -> [Int] {
        IntentMatching.indices(of: query, in: names)
    }

    // MARK: 完全一致 → 前方一致 → 部分一致の順

    func testExactBeatsPrefixAndPartial() {
        XCTAssertEqual(hits("家計簿", ["家計簿メモ", "家計簿", "私の家計簿"]), [1])
    }

    func testPrefixBeatsPartial() {
        XCTAssertEqual(hits("家計簿", ["家計簿メモ", "私の家計簿"]), [0])
    }

    func testPartialMatchesWhenNoExactOrPrefix() {
        XCTAssertEqual(hits("家計簿", ["私の家計簿メモ"]), [0])
    }

    /// 前方一致・部分一致は問い合わせ側が長い時も当たる(「家計簿2026」で「家計簿」)。
    func testPrefixAlsoMatchesWhenQueryIsLonger() {
        XCTAssertEqual(hits("家計簿2026", ["家計簿", "筋トレ"]), [0])
    }

    /// 同じ段で複数なら全部返して Siri に選ばせる。並びは `names` の順。
    func testAmbiguousReturnsAllHitsInNameOrder() {
        XCTAssertEqual(hits("家計", ["家計管理", "筋トレ", "家計簿"]), [0, 2])
    }

    func testNoMatchReturnsEmpty() {
        XCTAssertEqual(hits("日記", ["家計簿", "筋トレ"]), [])
    }

    // MARK: 末尾の「アプリ」「の方」

    func testTrailingAppSuffixIsIgnored() {
        XCTAssertEqual(hits("家計簿アプリ", ["家計簿", "筋トレ"]), [0])
        XCTAssertEqual(hits("家計簿の方", ["家計簿", "筋トレ"]), [0])
    }

    /// 「アプリ」だけは名前が残らないので候補なし。
    func testGenericAppWordAloneMatchesNothing() {
        XCTAssertEqual(hits("アプリ", ["家計簿", "筋トレ"]), [])
    }

    // MARK: 空白・記号・空の問い合わせ

    func testEmptyQueryReturnsEmpty() {
        XCTAssertEqual(hits("", ["家計簿", "筋トレ"]), [])
    }

    func testWhitespaceOnlyQueryReturnsEmpty() {
        XCTAssertEqual(hits("   ", ["家計簿"]), [])
        XCTAssertEqual(hits("\u{3000}", ["家計簿"]), [])
    }

    func testPunctuationOnlyQueryReturnsEmpty() {
        XCTAssertEqual(hits("！？", ["家計簿"]), [])
    }

    func testEmptyNameListReturnsEmpty() {
        XCTAssertEqual(hits("家計簿", []), [])
    }

    /// 名前が空のアプリは候補に入れない(空文字が何にでも前方一致してしまうのを避ける)。
    func testEmptyNameEntryIsSkipped() {
        XCTAssertEqual(hits("家計簿", ["", "家計簿"]), [1])
    }

    /// 発話に空白・句読点が混ざっても落として比べる(WhisperKit の揺れ)。
    func testWhitespaceAndPunctuationInQueryAreDropped() {
        XCTAssertEqual(hits(" 家 計 簿 ", ["家計簿", "筋トレ"]), [0])
        XCTAssertEqual(hits("日記・メモ", ["日記メモ", "家計簿"]), [0])
    }

    // MARK: 大文字小文字

    func testCaseInsensitiveForAscii() {
        XCTAssertEqual(hits("todo", ["ToDo", "家計簿"]), [0])
        XCTAssertEqual(hits("ToDo", ["todo", "家計簿"]), [0])
    }

    /// 大文字違いだけのアプリが 2 つあると同じキーになるので両方返る。
    func testCaseOnlyDuplicatesAreBothReturned() {
        XCTAssertEqual(hits("abc", ["ABC", "abc"]), [0, 1])
    }

    // MARK: ひらがな / カタカナ(現在の挙動を固定)

    /// 現在の実装はかなを畳まない。「かけいぼ」と言っても「家計簿」には当たらない。
    func testKanjiNameIsNotMatchedByKanaQuery() {
        XCTAssertEqual(hits("かけいぼ", ["家計簿", "筋トレ"]), [])
    }

    /// ひらがな ↔ カタカナも同一視しない(どちらの向きも不一致)。
    func testHiraganaAndKatakanaAreNotFolded() {
        XCTAssertEqual(hits("カケイボ", ["かけいぼ", "筋トレ"]), [])
        XCTAssertEqual(hits("かけいぼ", ["カケイボ", "筋トレ"]), [])
    }

    func testSameKanaScriptMatches() {
        XCTAssertEqual(hits("かけいぼ", ["かけいぼ", "筋トレ"]), [0])
    }

    // MARK: 全角 / 半角(現在の挙動を固定)

    /// 全角英数は半角に畳まない。`lowercased()` は幅を変えないので全角 ＡＢＣ → ａｂｃ のまま。
    func testFullWidthAsciiIsNotFoldedToHalfWidth() {
        XCTAssertEqual(hits("ＡＢＣ", ["ABC", "家計簿"]), [])
        XCTAssertEqual(hits("１２３", ["123", "家計簿"]), [])
    }

    /// 半角カタカナも全角カタカナと同一視しない。
    func testHalfWidthKatakanaIsNotFoldedToFullWidth() {
        XCTAssertEqual(hits("ｶﾀｶﾅ", ["カタカナ", "家計簿"]), [])
    }

    /// 全角同士・半角同士なら当たる。
    func testFullWidthMatchesFullWidth() {
        XCTAssertEqual(hits("ａｂｃ", ["ＡＢＣ", "家計簿"]), [0])
    }

    // MARK: 同名 2 つ

    /// 同じ名前のアプリ(entity 違いの記録先など)は完全一致でも両方返す。
    func testDuplicateNamesAreBothReturnedOnExactMatch() {
        XCTAssertEqual(hits("家計簿", ["家計簿", "家計簿", "筋トレ"]), [0, 1])
    }

    func testDuplicateNamesAreBothReturnedOnPrefixMatch() {
        XCTAssertEqual(hits("家計", ["家計簿", "家計簿", "筋トレ"]), [0, 1])
    }
}
