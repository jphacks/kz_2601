import XCTest
@testable import MetamonDSL

/// `AIOutputs` の数・日付の読み方(loneNumber・kanjiNumber・isoDay・hasYear・nextYearIfPast・rangeStart)を関数ごとに。
final class AIOutputsParsingTests: XCTestCase {
    // MARK: - loneNumber

    func testLoneNumberReadsSingleNumber() {
        XCTAssertEqual(AIOutputs.loneNumber("約 3.5 kg"), 3.5)
        XCTAssertEqual(AIOutputs.loneNumber("1,200 円"), 1200)
        XCTAssertEqual(AIOutputs.loneNumber("85 点"), 85)
        XCTAssertEqual(AIOutputs.loneNumber("-5 度"), -5)
        XCTAssertEqual(AIOutputs.loneNumber("１２０ 円"), 120) // 全角は半角にして読む
    }

    func testLoneNumberRejectsTwoOrNoNumbers() {
        XCTAssertNil(AIOutputs.loneNumber("85/100"))
        XCTAssertNil(AIOutputs.loneNumber("3.5kg を 2 回"))
        XCTAssertNil(AIOutputs.loneNumber("2026-11-20"))
        XCTAssertNil(AIOutputs.loneNumber("なし"))
        XCTAssertNil(AIOutputs.loneNumber(""))
    }

    // MARK: - kanjiNumber(万・千・億)

    private func assertUnreadable(_ s: String, file: StaticString = #filePath, line: UInt = #line) {
        guard case .some(.none) = AIOutputs.kanjiNumber(s) else {
            return XCTFail("\(s) は .some(nil)(読めないので空のまま)のはず", file: file, line: line)
        }
    }

    func testKanjiNumberReadsUnits() {
        XCTAssertEqual(AIOutputs.kanjiNumber("3万円"), .some(30_000))
        XCTAssertEqual(AIOutputs.kanjiNumber("約1.5万人"), .some(15_000))
        XCTAssertEqual(AIOutputs.kanjiNumber("1億2000万"), .some(120_000_000))
        XCTAssertEqual(AIOutputs.kanjiNumber("3千万"), .some(30_000_000))
        XCTAssertEqual(AIOutputs.kanjiNumber("5千"), .some(5_000))
        XCTAssertEqual(AIOutputs.kanjiNumber("1万5千"), .some(15_000))
        XCTAssertEqual(AIOutputs.kanjiNumber("2万300円"), .some(20_300))
        XCTAssertEqual(AIOutputs.kanjiNumber("-3万"), .some(-30_000))
        XCTAssertEqual(AIOutputs.kanjiNumber("２万"), .some(20_000)) // 全角数字
        XCTAssertEqual(AIOutputs.kanjiNumber("¥1,200万"), .some(12_000_000)) // ¥・桁区切りは外す
    }

    func testKanjiNumberWithoutUnitIsNil() {
        // 万・千・億が無ければ nil(ほかの読み方へ回す)。漢数字だけの「三」「十二」「二十五」「百」もここ
        for s in ["三", "十二", "二十五", "百", "120", "なし"] {
            XCTAssertNil(AIOutputs.kanjiNumber(s), s)
        }
    }

    func testKanjiNumberUnreadableStaysEmpty() {
        // 万・千・億はあるが読めない(漢数字との混在・範囲・以上)
        assertUnreadable("三千")
        assertUnreadable("二十五万")
        assertUnreadable("3万〜5万")
        assertUnreadable("3万円以上")
        assertUnreadable("千")
    }

    // MARK: - isoDay

    func testIsoDayKeepsOffsetDay() {
        XCTAssertEqual(AIOutputs.isoDay("2026-11-20T23:30:00-05:00"), "2026-11-20")
        XCTAssertEqual(AIOutputs.isoDay("2026-11-20T10:00Z"), "2026-11-20")
        XCTAssertEqual(AIOutputs.isoDay("2026-11-20T10:00:00.123+09:00"), "2026-11-20")
        XCTAssertEqual(AIOutputs.isoDay("2026-11-20T10:00"), "2026-11-20")
    }

    func testIsoDayRejectsBadForms() {
        XCTAssertNil(AIOutputs.isoDay("2026-11-20")) // 時刻が無い
        XCTAssertNil(AIOutputs.isoDay("2026-02-30T10:00:00Z")) // あり得ない日
        XCTAssertNil(AIOutputs.isoDay("11/20 10:00"))
        XCTAssertNil(AIOutputs.isoDay(""))
    }

    // MARK: - hasYear / nextYearIfPast

    func testHasYear() {
        XCTAssertTrue(AIOutputs.hasYear("2026/3/1"))
        XCTAssertTrue(AIOutputs.hasYear("2026年3月1日"))
        XCTAssertFalse(AIOutputs.hasYear("3/1"))
        XCTAssertFalse(AIOutputs.hasYear("3月1日 12:30"))
    }

    private func day(_ y: Int, _ m: Int, _ d: Int) -> Date {
        var cal = Calendar(identifier: .gregorian)
        cal.timeZone = FieldCoding.dateFormatter.timeZone
        return cal.date(from: DateComponents(year: y, month: m, day: d))!
    }

    func testNextYearIfPast() {
        let now = day(2026, 12, 15)
        // 30 日以上前 → 来年(12 月に「1/10」)
        XCTAssertEqual(AIOutputs.nextYearIfPast(day(2026, 1, 10), now: now), day(2027, 1, 10))
        // 30 日より近い過去・未来はそのまま(nil)
        XCTAssertNil(AIOutputs.nextYearIfPast(day(2026, 12, 1), now: now))
        XCTAssertNil(AIOutputs.nextYearIfPast(day(2026, 12, 20), now: now))
        // ちょうど 30 日前は来年にしない
        XCTAssertNil(AIOutputs.nextYearIfPast(now.addingTimeInterval(-30 * 86_400), now: now))
    }

    // MARK: - rangeStart

    func testRangeStartTakesStart() {
        XCTAssertEqual(AIOutputs.rangeStart("3/1〜3/5"), "3/1")
        XCTAssertEqual(AIOutputs.rangeStart("3/1～3/5"), "3/1") // 全角チルダ
        XCTAssertEqual(AIOutputs.rangeStart("18:00~20:00"), "18:00")
        XCTAssertEqual(AIOutputs.rangeStart("11/20 - 11/22"), "11/20")
        XCTAssertEqual(AIOutputs.rangeStart("11/20–11/22"), "11/20")
    }

    func testRangeStartWithoutSeparatorIsUnchanged() {
        // 「から」は区切りとして読まない(そのまま返す)。日付の「-」も区切らない
        XCTAssertEqual(AIOutputs.rangeStart("3月1日から"), "3月1日から")
        XCTAssertEqual(AIOutputs.rangeStart("2026-03-01"), "2026-03-01")
    }
}
