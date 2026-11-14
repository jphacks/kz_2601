import XCTest
@testable import MetamonDSL

/// すぐ記録の読み取りの部品(halfwidthDigits・matchOption・matchRef)を関数ごとに。
final class QuickRecordMatchTests: XCTestCase {
    // MARK: - halfwidthDigits

    func testHalfwidthDigitsConvertsDigitsAndMarks() {
        XCTAssertEqual(QuickRecord.halfwidthDigits("１２３"), "123")
        XCTAssertEqual(QuickRecord.halfwidthDigits("１，２００．５"), "1,200.5")
        XCTAssertEqual(QuickRecord.halfwidthDigits("￥８００"), "¥800")
        XCTAssertEqual(QuickRecord.halfwidthDigits("ランチ　８００円"), "ランチ 800円")
    }

    func testHalfwidthDigitsLeavesOthers() {
        XCTAssertEqual(QuickRecord.halfwidthDigits("三百円"), "三百円") // 漢数字は変えない
        XCTAssertEqual(QuickRecord.halfwidthDigits("コーヒー"), "コーヒー") // カタカナ・長音も変えない
        XCTAssertEqual(QuickRecord.halfwidthDigits("ＡＢＣ"), "ＡＢＣ") // 全角英字は対象外
        XCTAssertEqual(QuickRecord.halfwidthDigits("abc 123"), "abc 123")
        XCTAssertEqual(QuickRecord.halfwidthDigits(""), "")
    }

    // MARK: - matchOption

    func testMatchOptionExactAndPartial() {
        XCTAssertEqual(QuickRecord.matchOption(in: "ベンチプレス", options: ["スクワット", "ベンチプレス"]), "ベンチプレス")
        XCTAssertEqual(QuickRecord.matchOption(in: "今日はスクワット 60kg", options: ["スクワット", "ベンチプレス"]), "スクワット")
        // 大文字小文字は無視し、オプションの表記で返す
        XCTAssertEqual(QuickRecord.matchOption(in: "RUN 5km", options: ["Run", "Walk"]), "Run")
    }

    func testMatchOptionPrefersLongest() {
        XCTAssertEqual(QuickRecord.matchOption(in: "ベンチプレス 60kg", options: ["ベンチ", "ベンチプレス"]), "ベンチプレス")
        XCTAssertEqual(QuickRecord.matchOption(in: "ベンチプレス 60kg", options: ["ベンチプレス", "ベンチ"]), "ベンチプレス")
        // 同じ長さで 2 つ当たる時は先に書いた方
        XCTAssertEqual(QuickRecord.matchOption(in: "朝食と昼食", options: ["昼食", "朝食"]), "昼食")
    }

    func testMatchOptionNone() {
        XCTAssertNil(QuickRecord.matchOption(in: "ランニング", options: ["スクワット"]))
        XCTAssertNil(QuickRecord.matchOption(in: "ランニング", options: [""])) // 空の選択肢は当てない
        XCTAssertNil(QuickRecord.matchOption(in: "ランニング", options: []))
    }

    // MARK: - matchRef

    private let tanaka = QuickRecord.RefChoice(id: "p1", title: "田中太郎")
    private let sato = QuickRecord.RefChoice(id: "p2", title: "佐藤")

    func testMatchRefByNameInText() {
        XCTAssertEqual(QuickRecord.matchRef(in: "田中太郎さんとランチ 1200円", choices: [tanaka, sato]), tanaka)
        XCTAssertEqual(QuickRecord.matchRef(in: "佐藤 コーヒー", choices: [tanaka, sato]), sato)
        // 名前の一部だけ(「田中」)では当てない
        XCTAssertNil(QuickRecord.matchRef(in: "田中とランチ", choices: [tanaka]))
        XCTAssertNil(QuickRecord.matchRef(in: "ランチ", choices: [tanaka, sato]))
    }

    func testMatchRefPrefersLongestName() {
        let tanakaShort = QuickRecord.RefChoice(id: "p3", title: "田中")
        XCTAssertEqual(QuickRecord.matchRef(in: "田中太郎とランチ", choices: [tanakaShort, tanaka]), tanaka)
    }

    func testMatchRefSameNameTwiceTakesFirst() {
        let sato2 = QuickRecord.RefChoice(id: "p9", title: "佐藤")
        XCTAssertEqual(QuickRecord.matchRef(in: "佐藤とランチ", choices: [sato, sato2])?.id, "p2")
        XCTAssertEqual(QuickRecord.matchRef(in: "佐藤とランチ", choices: [sato2, sato])?.id, "p9")
    }

    func testMatchRefShortTitleOnlyAsWord() {
        let car = QuickRecord.RefChoice(id: "c1", title: "車")
        XCTAssertNil(QuickRecord.matchRef(in: "電車 300円", choices: [car])) // 「電車」に「車」を当てない
        XCTAssertEqual(QuickRecord.matchRef(in: "車 300円", choices: [car]), car) // 語として一致すれば当てる
    }
}
