#if !APP_STORE_REVIEW // 審査用ビルドはコイン(App 内課金)を持たない(#450)
import XCTest
import MetamonDSL
@testable import MetamonServices

/// アプリの枠の購入とコインの履歴の読み方(CoinSlotPurchase.decode・CoinLedgerPage.decode)(下書き stock7-ios-015)。
final class CoinLedgerDecodeTests: XCTestCase {
    private func data(_ s: String) -> Data { Data(s.utf8) }

    // MARK: - CoinLedgerPage

    func testNextCursorPresentEmptyAndMissing() throws {
        XCTAssertEqual(try CoinLedgerPage.decode(data(#"{"entries":[],"nextCursor":"abc"}"#)).nextCursor, "abc")
        XCTAssertNil(try CoinLedgerPage.decode(data(#"{"entries":[],"nextCursor":""}"#)).nextCursor)
        XCTAssertNil(try CoinLedgerPage.decode(data(#"{"entries":[]}"#)).nextCursor)
        XCTAssertNil(try CoinLedgerPage.decode(data(#"{"entries":[],"nextCursor":null}"#)).nextCursor)
        XCTAssertNil(try CoinLedgerPage.decode(data(#"{"entries":[],"nextCursor":5}"#)).nextCursor, "文字でなければ続き無し")
    }

    func testUnknownKindIsKeptAsIs() throws {
        let page = try CoinLedgerPage.decode(data(#"{"entries":[{"id":"a","kind":"future_kind","amount":5},{"id":"b","amount":-1}]}"#))
        XCTAssertEqual(page.entries.map(\.kind), ["future_kind", ""], "知らない種類はそのまま・kind が無ければ空")
        XCTAssertEqual(page.entries.map(\.delta), [5, -1])
    }

    func testMissingOrBadDatesAreNilButRowsStay() throws {
        let json = #"""
        {"entries":[
          {"id":"a","kind":"purchase","amount":1},
          {"id":"b","kind":"purchase","amount":2,"at":"not a date"},
          {"id":"c","kind":"purchase","amount":3,"at":12345},
          {"id":"d","kind":"purchase","amount":4,"at":"2026-11-10T01:00:00Z"},
          {"id":"e","kind":"purchase","amount":5,"at":"2026-11-10T01:00:00.250Z"}
        ]}
        """#
        let page = try CoinLedgerPage.decode(data(json))
        XCTAssertEqual(page.entries.count, 5)
        XCTAssertEqual(page.entries.map { $0.at == nil }, [true, true, true, false, false])
        let d = try XCTUnwrap(page.entries[3].at), e = try XCTUnwrap(page.entries[4].at)
        XCTAssertEqual(e.timeIntervalSince(d), 0.25, accuracy: 0.001, "小数秒つきも読む")
        XCTAssertEqual(d.timeIntervalSince1970, 1_794_272_400, accuracy: 0.5)
    }

    func testMissingIdIsMadeFromIndexAndTime() throws {
        let page = try CoinLedgerPage.decode(data(#"{"entries":[{"kind":"adjust","amount":1},{"kind":"adjust","amount":2,"at":"1970-01-01T00:00:10Z"}]}"#))
        XCTAssertEqual(page.entries.map(\.id), ["0-0.0", "1-10.0"], "id が無い行は 位置-時刻 で作り、重ならない")
    }

    func testAmountBeatsDeltaAndAcceptsNumericStrings() throws {
        let page = try CoinLedgerPage.decode(data(#"{"entries":[{"id":"a","amount":-3,"delta":9},{"id":"b","delta":"7"},{"id":"c","amount":"x","delta":2},{"id":"d","amount":"x"}]}"#))
        XCTAssertEqual(page.entries.map(\.id), ["a", "b", "c"], "どちらも読めない d だけ捨てる")
        XCTAssertEqual(page.entries.map(\.delta), [-3, 7, 2], "amount が先・読めなければ delta")
    }

    func testRowsWithoutAmountAreDroppedAndNonObjectsSkipped() throws {
        let page = try CoinLedgerPage.decode(data(#"{"entries":[1,"x",null,[],{"id":"ok","amount":10},{"id":"no-amount"}]}"#))
        XCTAssertEqual(page.entries.map(\.id), ["ok"])
    }

    func testItemsKeyAndBareArrayAreAccepted() throws {
        XCTAssertEqual(try CoinLedgerPage.decode(data(#"{"items":[{"id":"a","amount":1}]}"#)).entries.map(\.id), ["a"])
        XCTAssertEqual(try CoinLedgerPage.decode(data(#"[{"id":"b","amount":2}]"#)).entries.map(\.id), ["b"])
        XCTAssertEqual(try CoinLedgerPage.decode(data(#"{"entries":[{"id":"c","amount":3}],"items":[{"id":"z","amount":9}]}"#)).entries.map(\.id), ["c"], "entries が先")
    }

    func testEmptyAndOddRootsGiveEmptyPage() throws {
        XCTAssertEqual(try CoinLedgerPage.decode(data("{}")), CoinLedgerPage(entries: []))
        XCTAssertEqual(try CoinLedgerPage.decode(data("[]")), CoinLedgerPage(entries: []))
        XCTAssertEqual(try CoinLedgerPage.decode(data("5")), CoinLedgerPage(entries: []))
        XCTAssertThrowsError(try CoinLedgerPage.decode(data("not json")))
    }

    func testOperationAndAppIdAreOptional() throws {
        let page = try CoinLedgerPage.decode(data(#"{"entries":[{"id":"a","kind":"spend_ai","amount":-2,"op":"text","appId":"app9"},{"id":"b","kind":"purchase","amount":2}]}"#))
        XCTAssertEqual(page.entries[0].operation, "text")
        XCTAssertEqual(page.entries[0].appId, "app9")
        XCTAssertNil(page.entries[1].operation)
        XCTAssertNil(page.entries[1].appId)
    }

    // MARK: - CoinSlotPurchase

    func testSlotPurchaseServerShape() throws {
        let json = #"{"bought":true,"appSlots":3,"coins":50,"wallet":{"mode":"enforce","purchased":40,"appSlots":3}}"#
        let p = try CoinSlotPurchase.decode(data(json))
        XCTAssertTrue(p.bought)
        XCTAssertEqual(p.appSlots, 3)
        XCTAssertEqual(p.coins, 50)
        XCTAssertEqual(p.wallet.balance.purchased, 40)
        XCTAssertEqual(p.wallet.mode, "enforce")
    }

    func testSlotPurchaseAlreadyBoughtMeansNothingCharged() throws {
        let p = try CoinSlotPurchase.decode(data(#"{"bought":false,"appSlots":2,"coins":50,"wallet":{"purchased":90}}"#))
        XCTAssertFalse(p.bought)
        XCTAssertEqual(p.wallet.balance.purchased, 90)
    }

    func testSlotPurchaseDefaultsWhenFieldsMissingOrWrongType() throws {
        let p = try CoinSlotPurchase.decode(data(#"{"bought":"yes","appSlots":"x"}"#))
        XCTAssertFalse(p.bought, "型が違えば買えていない扱い")
        XCTAssertEqual(p.appSlots, 0)
        XCTAssertEqual(p.coins, 0)
        XCTAssertEqual(p.wallet, CoinWallet(object: [:]), "wallet が無ければ空の財布")
        XCTAssertEqual(p.wallet.balance.available, 0)
        let q = try CoinSlotPurchase.decode(data(#"{"bought":true,"appSlots":"4","coins":"20","wallet":[]}"#))
        XCTAssertEqual(q.appSlots, 4)
        XCTAssertEqual(q.coins, 20)
        XCTAssertEqual(q.wallet, CoinWallet(object: [:]))
    }

    func testSlotPurchaseRejectsNonObject() {
        XCTAssertThrowsError(try CoinSlotPurchase.decode(data("[]"))) { XCTAssertEqual($0 as? CoinDecodeError, .notAnObject) }
        XCTAssertThrowsError(try CoinSlotPurchase.decode(data("null"))) { XCTAssertEqual($0 as? CoinDecodeError, .notAnObject) }
    }
}
#endif
