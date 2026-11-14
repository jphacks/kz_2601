import XCTest
@testable import MetamonRenderer

/// 交通系 IC カードの履歴のバイト列を残高・履歴に変える所(#2246)。実機・実カード無しで確かめられる範囲だけ。
final class FeliCaReaderTests: XCTestCase {
    /// 履歴 1 件(16 バイト)を組み立てる。日付・残高の並びはカードと同じにする。
    private func block(
        process: UInt8,
        year: Int = 26,
        month: Int = 9,
        day: Int = 10,
        balance: Int = 1000,
        terminal: UInt8 = 0x16
    ) -> Data {
        let packed = UInt16(year) << 9 | UInt16(month) << 5 | UInt16(day)
        let yen = UInt16(balance)
        return Data([
            terminal, process, 0x00, 0x02,
            UInt8(packed >> 8), UInt8(packed & 0xFF),
            0x00, 0x7A, 0x00, 0x91,
            UInt8(yen & 0xFF), UInt8(yen >> 8),
            0x00, 0x00, 0x01, 0x00,
        ])
    }

    // MARK: 日付

    func testDayDecodesYearMonthDay() {
        XCTAssertEqual(FeliCaTransitCodec.day(high: 0x35, low: 0x2A), TransitDay(year: 2026, month: 9, day: 10))
        XCTAssertEqual(FeliCaTransitCodec.day(high: 0x35, low: 0x29), TransitDay(year: 2026, month: 9, day: 9))
    }

    /// 年は 2000 年からの数え(7 ビット)。繰り上がり(12 月 → 1 月)で年が 1 つ進む。
    func testDayYearCarryOver() {
        let december = FeliCaTransitCodec.day(high: UInt8((26 << 9 | 12 << 5 | 31) >> 8), low: UInt8((26 << 9 | 12 << 5 | 31) & 0xFF))
        let january = FeliCaTransitCodec.day(high: UInt8((27 << 9 | 1 << 5 | 1) >> 8), low: UInt8((27 << 9 | 1 << 5 | 1) & 0xFF))
        XCTAssertEqual(december, TransitDay(year: 2026, month: 12, day: 31))
        XCTAssertEqual(january, TransitDay(year: 2027, month: 1, day: 1))
        XCTAssertEqual(january?.date?.timeIntervalSince(december?.date ?? .distantPast), 86400)

        // 7 ビットなので 2000〜2127 年まで表せる
        XCTAssertEqual(FeliCaTransitCodec.day(high: UInt8((0 << 9 | 1 << 5 | 1) >> 8), low: UInt8((0 << 9 | 1 << 5 | 1) & 0xFF)),
                       TransitDay(year: 2000, month: 1, day: 1))
        XCTAssertEqual(FeliCaTransitCodec.day(high: UInt8((127 << 9 | 12 << 5 | 31) >> 8), low: UInt8((127 << 9 | 12 << 5 | 31) & 0xFF)),
                       TransitDay(year: 2127, month: 12, day: 31))
    }

    func testDayRejectsImpossibleValues() {
        XCTAssertNil(FeliCaTransitCodec.day(high: 0x00, low: 0x00)) // 未使用の枠
        XCTAssertNil(FeliCaTransitCodec.day(high: UInt8((26 << 9 | 13 << 5 | 1) >> 8), low: UInt8((26 << 9 | 13 << 5 | 1) & 0xFF))) // 13 月
        XCTAssertNil(FeliCaTransitCodec.day(high: UInt8((26 << 9 | 9 << 5 | 0) >> 8), low: UInt8((26 << 9 | 9 << 5 | 0) & 0xFF))) // 0 日
    }

    /// 日本時間のその日の 0 時になる。
    func testDayDateIsMidnightInJapan() throws {
        let date = try XCTUnwrap(TransitDay(year: 2026, month: 9, day: 10).date)
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = try XCTUnwrap(TimeZone(identifier: "Asia/Tokyo"))
        let parts = calendar.dateComponents([.year, .month, .day, .hour, .minute], from: date)
        XCTAssertEqual(parts.year, 2026)
        XCTAssertEqual(parts.month, 9)
        XCTAssertEqual(parts.day, 10)
        XCTAssertEqual(parts.hour, 0)
        XCTAssertEqual(parts.minute, 0)
    }

    // MARK: 残高・種類

    func testBalanceIsLittleEndian() {
        XCTAssertEqual(FeliCaTransitCodec.balance(low: 0xB0, high: 0x09), 2480)
        XCTAssertEqual(FeliCaTransitCodec.balance(low: 0x00, high: 0x00), 0)
        XCTAssertEqual(FeliCaTransitCodec.balance(low: 0xFF, high: 0xFF), 65535)
    }

    func testKindMapping() {
        XCTAssertEqual(FeliCaTransitCodec.kind(processCode: 0x01), .fare)
        XCTAssertEqual(FeliCaTransitCodec.kind(processCode: 0x02), .charge)
        XCTAssertEqual(FeliCaTransitCodec.kind(processCode: 0x03), .ticket)
        XCTAssertEqual(FeliCaTransitCodec.kind(processCode: 0x04), .settlement)
        XCTAssertEqual(FeliCaTransitCodec.kind(processCode: 0x46), .purchase)
        XCTAssertEqual(FeliCaTransitCodec.kind(processCode: 0x0D), .bus)
        XCTAssertEqual(FeliCaTransitCodec.kind(processCode: 0x11), .issued)
        XCTAssertEqual(FeliCaTransitCodec.kind(processCode: 0x14), .entry)
        XCTAssertEqual(FeliCaTransitCodec.kind(processCode: 0x7E), .unknown)
    }

    // MARK: 1 件ぶん

    func testTransactionFromBlock() throws {
        let entry = try XCTUnwrap(FeliCaTransitCodec.transaction(fromBlock: block(process: 0x01, balance: 2480)))
        XCTAssertEqual(entry.day, TransitDay(year: 2026, month: 9, day: 10))
        XCTAssertEqual(entry.kind, .fare)
        XCTAssertEqual(entry.balance, 2480)
        XCTAssertNil(entry.amount) // 1 件だけでは増減は分からない
    }

    func testTransactionRejectsUnusableBlocks() {
        XCTAssertNil(FeliCaTransitCodec.transaction(fromBlock: Data(repeating: 0, count: 16))) // 未使用の枠
        XCTAssertNil(FeliCaTransitCodec.transaction(fromBlock: Data()))
        XCTAssertNil(FeliCaTransitCodec.transaction(fromBlock: Data(repeating: 0x11, count: 15))) // 短い
        XCTAssertNil(FeliCaTransitCodec.transaction(fromBlock: Data(repeating: 0x11, count: 17))) // 長い
    }

    /// 日付が読めない記録でも、残高と種類は読む。
    func testTransactionKeepsBalanceWhenDayIsBroken() throws {
        var bytes = [UInt8](block(process: 0x02, balance: 500))
        bytes[4] = 0x00
        bytes[5] = 0x00
        let entry = try XCTUnwrap(FeliCaTransitCodec.transaction(fromBlock: Data(bytes)))
        XCTAssertNil(entry.day)
        XCTAssertEqual(entry.kind, .charge)
        XCTAssertEqual(entry.balance, 500)
    }

    // MARK: 1 枚ぶん

    /// カードから読む順は新しい順。返すのは古い順で、増減は残高の差。
    func testCardFromBlocksIsOldestFirstWithAmounts() throws {
        let newestFirst = [
            block(process: 0x01, day: 10, balance: 2480), // 運賃 -140
            block(process: 0x46, day: 10, balance: 2620), // 物販 -160
            block(process: 0x01, day: 9, balance: 2780), // 運賃 -220
            block(process: 0x02, day: 9, balance: 3000), // チャージ(ここより前は分からない)
        ]
        let card = try XCTUnwrap(FeliCaTransitCodec.card(fromBlocks: newestFirst))
        XCTAssertEqual(card.balance, 2480)
        XCTAssertEqual(card.history.map(\.balance), [3000, 2780, 2620, 2480])
        XCTAssertEqual(card.history.map(\.kind), [.charge, .fare, .purchase, .fare])
        XCTAssertEqual(card.history.map(\.amount), [nil, -220, -160, -140])
        XCTAssertEqual(card.history.map { $0.day?.day }, [9, 9, 10, 10])
    }

    /// チャージは +、支払いは -。
    func testChargeIsPositiveAndPaymentIsNegative() throws {
        let card = try XCTUnwrap(FeliCaTransitCodec.card(fromBlocks: [
            block(process: 0x02, balance: 4000), // チャージ +3000
            block(process: 0x01, balance: 1000), // 運賃 -500
            block(process: 0x01, balance: 1500),
        ]))
        XCTAssertEqual(card.balance, 4000)
        XCTAssertEqual(card.history.map(\.amount), [nil, -500, 3000])
        XCTAssertEqual(card.history.map(\.kind), [.fare, .fare, .charge])
    }

    func testCardSkipsUnusedBlocks() throws {
        let card = try XCTUnwrap(FeliCaTransitCodec.card(fromBlocks: [
            block(process: 0x01, balance: 800),
            Data(repeating: 0, count: 16),
            Data(),
        ]))
        XCTAssertEqual(card.balance, 800)
        XCTAssertEqual(card.history.count, 1)
        XCTAssertNil(card.history[0].amount)
    }

    func testCardIsNilWhenNothingIsReadable() {
        XCTAssertNil(FeliCaTransitCodec.card(fromBlocks: []))
        XCTAssertNil(FeliCaTransitCodec.card(fromBlocks: [Data(repeating: 0, count: 16), Data()]))
    }

    /// 年をまたぐ履歴でも、古い順の並びと増減は崩れない。
    func testCardAcrossYearBoundary() throws {
        let card = try XCTUnwrap(FeliCaTransitCodec.card(fromBlocks: [
            block(process: 0x01, year: 27, month: 1, day: 3, balance: 1200),
            block(process: 0x02, year: 26, month: 12, day: 31, balance: 1400),
        ]))
        XCTAssertEqual(card.history.map(\.day), [
            TransitDay(year: 2026, month: 12, day: 31),
            TransitDay(year: 2027, month: 1, day: 3),
        ])
        XCTAssertEqual(card.history.map(\.amount), [nil, -200])
    }

    // MARK: 中身を漏らさない

    func testCardDoesNotLeakItsContentsInDescription() throws {
        let card = try XCTUnwrap(FeliCaTransitCodec.card(fromBlocks: FakeTransitCardReader.sampleBlocks))
        for text in [card.description, card.debugDescription, "\(card)"] {
            XCTAssertFalse(text.contains("2480"))
            XCTAssertFalse(text.contains("fare"))
        }
        let entry = card.history[0]
        for text in [entry.description, entry.debugDescription, "\(entry)"] {
            XCTAssertFalse(text.contains("3000"))
            XCTAssertFalse(text.contains("charge"))
        }
    }

    // MARK: Info.plist の申告

    func testHasTransitSystemCode() {
        let key = "com.apple.developer.nfc.readersession.felica.systemcodes"
        XCTAssertTrue(FeliCaTransitCodec.hasTransitSystemCode(in: [key: ["0003"]]))
        XCTAssertTrue(FeliCaTransitCodec.hasTransitSystemCode(in: [key: ["8008", "0003"]]))
        XCTAssertTrue(FeliCaTransitCodec.hasTransitSystemCode(in: [key: ["0003"]])) // 16 進の大文字小文字は区別しない
        XCTAssertFalse(FeliCaTransitCodec.hasTransitSystemCode(in: [key: ["8008"]]))
        XCTAssertFalse(FeliCaTransitCodec.hasTransitSystemCode(in: [key: []]))
        XCTAssertFalse(FeliCaTransitCodec.hasTransitSystemCode(in: [:]))
        XCTAssertFalse(FeliCaTransitCodec.hasTransitSystemCode(in: nil))
    }

    // MARK: 口

    func testFakeReaderReadsTheSampleCard() async throws {
        let reader = FakeTransitCardReader.sample
        let card = try await reader.read()
        XCTAssertEqual(card.balance, 2480)
        XCTAssertEqual(card.history.count, 4)
        XCTAssertEqual(card.history.map(\.amount), [nil, -220, -160, -140])
        XCTAssertEqual(reader.readCount, 1)
    }

    func testFakeReaderThrowsWhenUnavailableOrEmpty() async {
        let off = FakeTransitCardReader(isAvailable: false, card: TransitCard(balance: 0, history: []))
        do {
            _ = try await off.read()
            XCTFail("使えないはず")
        } catch {
            XCTAssertEqual(error as? TransitCardError, .unavailable)
        }

        let blank = FakeTransitCardReader(card: nil)
        do {
            _ = try await blank.read()
            XCTFail("履歴が無いはず")
        } catch {
            XCTAssertEqual(error as? TransitCardError, .empty)
        }
    }

    /// Core NFC が無い所(Mac)では読めない。
    func testLiveReaderOnPlatformsWithoutCoreNFC() async {
        #if !canImport(CoreNFC)
        let reader = LiveFeliCaTransitReader()
        XCTAssertFalse(reader.isAvailable)
        do {
            _ = try await reader.read()
            XCTFail("使えないはず")
        } catch {
            XCTAssertEqual(error as? TransitCardError, .unavailable)
        }
        #endif
    }

    func testErrorMessagesAreDistinct() {
        let all: [TransitCardError] = [.unavailable, .canceled, .notTransitCard, .empty, .failed]
        let messages = all.map { $0.errorDescription ?? "" }
        XCTAssertFalse(messages.contains(""))
        XCTAssertEqual(Set(messages).count, all.count)
    }

    func testServiceAndSystemCodes() {
        XCTAssertEqual(FeliCaTransitCodec.historyServiceCode, 0x090F)
        XCTAssertEqual(FeliCaTransitCodec.transitSystemCode, 0x0003)
        XCTAssertEqual(FeliCaTransitCodec.transitSystemCodeText, "0003")
        XCTAssertEqual(FeliCaTransitCodec.blockSize, 16)
    }
}
