import XCTest
@testable import MetamonDSL

final class FieldCodingTests: XCTestCase {
    private func field(_ type: String) throws -> Field {
        try JSONDecoder().decode(Field.self, from: Data(#"{"id":"f","name":"F","type":"\#(type)"}"#.utf8))
    }

    /// #121: 未入力(.null)は `.some(.null)` ではなく nil を返し、payload からキーごと落ちること。
    func testNormalizeDropsNull() throws {
        for type in ["photo", "location", "string", "text", "enum", "date", "datetime", "integer", "currency", "number"] {
            XCTAssertNil(FieldCoding.normalize(.null, for: try field(type)), "type=\(type)")
        }
    }

    func testNormalizeDropsEmptyString() throws {
        for type in ["photo", "location", "string", "integer"] {
            XCTAssertNil(FieldCoding.normalize(.string(""), for: try field(type)), "type=\(type)")
        }
    }

    func testNormalizeKeepsValues() throws {
        XCTAssertEqual(FieldCoding.normalize(.string("1,234"), for: try field("currency")), .number(1234))
        XCTAssertEqual(FieldCoding.normalize(.string("abc"), for: try field("string")), .string("abc"))
        XCTAssertEqual(FieldCoding.normalize(.string("35.0,139.0"), for: try field("location")), .string("35.0,139.0"))
        XCTAssertEqual(FieldCoding.normalize(.null, for: try field("boolean")), .bool(false))
    }

    /// #140: 一覧の日付は生の "2026-10-24" ではなく曜日付きで。今年なら年を省く。
    func testDisplayDateJaAndEn() throws {
        let now = try XCTUnwrap(FieldCoding.dateFormatter.date(from: "2026-06-01"))
        let ja = Locale(identifier: "ja_JP"), en = Locale(identifier: "en_US")
        let d = try field("date")
        XCTAssertEqual(FieldCoding.display(.string("2026-10-24"), field: d, locale: ja, now: now), "10/24(土)")
        XCTAssertEqual(FieldCoding.display(.string("2025-10-24"), field: d, locale: ja, now: now), "2025/10/24(金)")
        XCTAssertEqual(FieldCoding.display(.string("2026-10-24"), field: d, locale: en, now: now), "Sat, Oct 24")
        XCTAssertEqual(FieldCoding.display(.string("2025-10-24"), field: d, locale: en, now: now), "Fri, Oct 24, 2025")
        // 解釈できない値はそのまま
        XCTAssertEqual(FieldCoding.display(.string("来週"), field: d, locale: ja, now: now), "来週")
        XCTAssertEqual(FieldCoding.display(.null, field: d, locale: ja, now: now), "")
    }

    /// 端末の暦が和暦・仏暦でも、日本語の固定書式は西暦で出す(和暦だと「0007/10/24」になっていた)。
    func testDisplayDateJaIgnoresNonGregorianCalendar() throws {
        let now = try XCTUnwrap(FieldCoding.dateFormatter.date(from: "2026-06-01"))
        let d = try field("date")
        for id in ["ja_JP@calendar=japanese", "ja_JP@calendar=buddhist"] {
            let locale = Locale(identifier: id)
            XCTAssertEqual(FieldCoding.display(.string("2025-10-24"), field: d, locale: locale, now: now), "2025/10/24(金)", id)
            XCTAssertEqual(FieldCoding.display(.string("2026-10-24"), field: d, locale: locale, now: now), "10/24(土)", id)
        }
    }

    func testDisplayDatetimeHasWeekdayAndTime() throws {
        let now = try XCTUnwrap(FieldCoding.dateFormatter.date(from: "2026-06-01"))
        let dt = try field("datetime")
        let s = FieldCoding.display(.string("2026-10-24T12:30:00Z"), field: dt, locale: Locale(identifier: "ja_JP"), now: now)
        XCTAssertTrue(s.hasPrefix("10/2"), s)
        XCTAssertTrue(s.contains(":30"), s)
        XCTAssertFalse(s.contains("2026"), s)
    }

    /// #555: あり得ない日時は繰り上げて読まない。正しい形(オフセット・小数秒・1 桁の月日・うるう日)は読む。
    func testDateRejectsImpossibleDatetimes() {
        for s in ["2026-02-30T10:00:00Z", "2026-10-24T24:00:00Z", "2026-10-24T10:60:00+09:00", "2026-12-31T23:59:60Z",
                  "2027-02-29T00:00:00+0900", "2026-02-30T10:00:00.5Z", "2026-02-30", "2027-02-29", "2026-2-30", "0-01-01"] {
            XCTAssertNil(FieldCoding.date(from: .string(s)), s)
        }
        XCTAssertEqual(FieldCoding.date(from: "2026-10-24T10:00:00Z")?.timeIntervalSince1970, 1_792_836_000)
        XCTAssertEqual(FieldCoding.date(from: "2026-10-24T19:00:00+09:00")?.timeIntervalSince1970, 1_792_836_000)
        XCTAssertEqual(FieldCoding.date(from: "2026-10-24T05:00:00-0500")?.timeIntervalSince1970, 1_792_836_000)
        XCTAssertEqual(FieldCoding.date(from: "2026-10-24T10:00:00.250Z")?.timeIntervalSince1970, 1_792_836_000.25)
        XCTAssertEqual(FieldCoding.date(from: "2028-02-29T23:59:59-05:00")?.timeIntervalSince1970, 1_835_499_599)
        XCTAssertNotNil(FieldCoding.date(from: "2028-02-29"))
        XCTAssertNotNil(FieldCoding.date(from: "2026-1-5"))
    }
    private func currencyField(unit: String?) throws -> Field {
        let u = unit.map { #","unit":"\#($0)""# } ?? ""
        return try JSONDecoder().decode(Field.self, from: Data(#"{"id":"f","name":"F","type":"currency"\#(u)}"#.utf8))
    }

    /// #150: unit が通貨そのもの(円・¥・JPY・$ 等)なら ¥ と重ねない。
    func testCurrencySuffixDropsCurrencyUnits() {
        for unit in ["円", "¥", "￥", "JPY", "jpy", "yen", "$", "USD", "dollars", " 円 ", "", nil] as [String?] {
            XCTAssertNil(FieldCoding.currencySuffix(unit: unit), "unit=\(unit ?? "nil")")
        }
    }

    /// #150: 「円/時」は通貨部分だけ落として「/時」を残す。通貨と無関係な unit はそのまま。
    func testCurrencySuffixKeepsRest() {
        XCTAssertEqual(FieldCoding.currencySuffix(unit: "円/時"), "/時")
        XCTAssertEqual(FieldCoding.currencySuffix(unit: "円 / 月"), "/ 月")
        XCTAssertEqual(FieldCoding.currencySuffix(unit: "yen per hour"), "per hour")
        XCTAssertEqual(FieldCoding.currencySuffix(unit: "/h"), "/h")
        XCTAssertEqual(FieldCoding.currencySuffix(unit: "per month"), "per month")
    }

    /// #150: 一覧の金額は「¥800」(「¥800 円」にしない)。per 単位は後ろに付ける。
    func testDisplayCurrencyWithUnit() throws {
        let ja = Locale(identifier: "ja_JP")
        XCTAssertEqual(FieldCoding.display(.number(800), field: try currencyField(unit: nil), locale: ja), "¥800")
        XCTAssertEqual(FieldCoding.display(.number(800), field: try currencyField(unit: "円"), locale: ja), "¥800")
        XCTAssertEqual(FieldCoding.display(.number(1200), field: try currencyField(unit: "JPY"), locale: ja), "¥1,200")
        XCTAssertEqual(FieldCoding.display(.number(15), field: try currencyField(unit: "$"), locale: ja), "¥15")
        XCTAssertEqual(FieldCoding.display(.number(1200), field: try currencyField(unit: "円/時"), locale: ja), "¥1,200/時")
        XCTAssertEqual(FieldCoding.display(.number(1200), field: try currencyField(unit: "yen per hour"), locale: ja), "¥1,200 per hour")
        XCTAssertEqual(FieldCoding.display(.number(-500), field: try currencyField(unit: "円"), locale: ja), "-¥500")
    }

    /// #150: boolean / photo の語は渡された words(Renderer が L() で引く)を使う。既定は日本語。
    func testDisplayBooleanWords() throws {
        let b = try field("boolean"), p = try field("photo")
        XCTAssertEqual(FieldCoding.display(.bool(true), field: b), "はい")
        XCTAssertEqual(FieldCoding.display(.bool(false), field: b), "いいえ")
        let en = FieldCoding.Words(yes: "Yes", no: "No", photo: "Photo")
        XCTAssertEqual(FieldCoding.display(.bool(true), field: b, words: en), "Yes")
        XCTAssertEqual(FieldCoding.display(.bool(false), field: b, words: en), "No")
        XCTAssertEqual(FieldCoding.display(.string("https://x/y.jpg"), field: p, words: en), "Photo")
        XCTAssertEqual(FieldCoding.display(.null, field: p, words: en), "")
    }
}
