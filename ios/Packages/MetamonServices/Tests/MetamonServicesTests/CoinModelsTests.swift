#if !APP_STORE_REVIEW // 審査用ビルドはコイン(App 内課金)を持たない(#450)
import XCTest
import MetamonDSL
@testable import MetamonServices

/// コインの読み方(CoinWallet.decodeCredit・CoinEstimates の鍵の揺れ・CoinPlanConfig の allowance)(下書き stock7-ios-014)。
final class CoinModelsTests: XCTestCase {
    private func data(_ s: String) -> Data { Data(s.utf8) }

    // MARK: - decodeCredit

    func testDecodeCreditReadsWalletInsideResponse() throws {
        let json = #"""
        {"credited":true,"transactionId":"t1","productId":"p.s","coins":100,
         "wallet":{"mode":"enforce","purchased":150,"grant":0,"initial":10,"reserved":0,"appSlots":2}}
        """#
        let w = try CoinWallet.decodeCredit(data(json))
        XCTAssertEqual(w.mode, "enforce")
        XCTAssertEqual(w.balance.purchased, 150, "トップの coins ではなく wallet の中を読む")
        XCTAssertEqual(w.balance.initial, 10)
        XCTAssertEqual(w.appSlots, 2)
    }

    func testDecodeCreditFallsBackToTopLevelWhenNoWallet() throws {
        let w = try CoinWallet.decodeCredit(data(#"{"mode":"log","purchased":5,"grant":3}"#))
        XCTAssertEqual(w.mode, "log")
        XCTAssertEqual(w.balance.available, 8)
    }

    func testDecodeCreditWalletOfWrongTypeFallsBackToTopLevel() throws {
        let w = try CoinWallet.decodeCredit(data(#"{"wallet":"x","purchased":4}"#))
        XCTAssertEqual(w.balance.purchased, 4)
    }

    func testDecodeCreditRejectsNonObjectAndGarbage() {
        XCTAssertThrowsError(try CoinWallet.decodeCredit(data("[]"))) { XCTAssertEqual($0 as? CoinDecodeError, .notAnObject) }
        XCTAssertThrowsError(try CoinWallet.decodeCredit(data(#""wallet""#))) { XCTAssertEqual($0 as? CoinDecodeError, .notAnObject) }
        XCTAssertThrowsError(try CoinWallet.decodeCredit(data("not json")))
    }

    func testWalletAppSlotsNeverNegative() throws {
        XCTAssertEqual(try CoinWallet.decode(data(#"{"appSlots":-3}"#)).appSlots, 0)
        XCTAssertEqual(try CoinWallet.decode(data(#"{"appSlots":"4"}"#)).appSlots, 4)
        XCTAssertEqual(try CoinWallet.decode(data("{}")).appSlots, 0)
    }

    // MARK: - CoinEstimates

    func testEstimatesKeysIgnoreCaseUnderscoreAndDash() {
        let e = CoinEstimates(["Generate": 30, "AI_BUTTON": 2, "ed-it": 7, "CHAT": 1])
        XCTAssertEqual(e.coins(for: .generate), 30)
        XCTAssertEqual(e.coins(for: .text), 2)
        XCTAssertEqual(e.coins(for: .edit), 7)
        XCTAssertEqual(e.coins(for: .chat), 1)
        XCTAssertEqual(e.values["aibutton"], 2, "鍵は正規化して持つ")
    }

    func testEstimatesOldNamesAndPriorityOfFirstKey() {
        XCTAssertEqual(CoinEstimates(["create": 12]).coins(for: .generate), 12, "古い名前(create)も受ける")
        XCTAssertEqual(CoinEstimates(["aichat": 5]).coins(for: .text), 5)
        XCTAssertEqual(CoinEstimates(["text": 3, "aibutton": 9]).coins(for: .text), 3, "先の名前を優先")
        XCTAssertEqual(CoinEstimates(["text": 0, "aibutton": 9]).coins(for: .text), 9, "0 以下は飛ばして次の名前を見る")
    }

    func testEstimatesZeroNegativeAndMissingAreNil() {
        let e = CoinEstimates(["generate": 0, "edit": -4])
        XCTAssertNil(e.coins(for: .generate))
        XCTAssertNil(e.coins(for: .edit))
        XCTAssertNil(e.coins(for: .chat))
        XCTAssertFalse(e.isEmpty, "値が使えなくても鍵があれば空ではない")
        XCTAssertTrue(CoinEstimates().isEmpty)
    }

    func testEstimatesFromJSONAcceptsNumberStringCoinsAndMedian() {
        let json: JSONValue = .object([
            "generate": .number(30),
            "edit": .string("11"),
            "chat": .object(["coins": .number(2)]),
            "text": .object(["median": .number(6)]),
            "bad": .object(["x": .number(1)]),
            "junk": .array([]),
        ])
        let e = CoinEstimates(json: json)
        XCTAssertEqual(e.coins(for: .generate), 30)
        XCTAssertEqual(e.coins(for: .edit), 11)
        XCTAssertEqual(e.coins(for: .chat), 2)
        XCTAssertEqual(e.coins(for: .text), 6)
        XCTAssertEqual(e.values.count, 4, "読めない形は捨てる")
        XCTAssertTrue(CoinEstimates(json: nil).isEmpty)
        XCTAssertTrue(CoinEstimates(json: .string("x")).isEmpty)
    }

    func testEstimatesCoinsBeatsMedianInsideObject() {
        let e = CoinEstimates(json: .object(["generate": .object(["coins": .number(8), "median": .number(40)])]))
        XCTAssertEqual(e.coins(for: .generate), 8)
    }

    // MARK: - CoinPlanConfig の allowance

    func testConfigAllowanceKeepsPositiveNumbersAndLimitObjects() throws {
        let json = #"{"mode":"enforce","allowance":{"runs":50,"recipients":{"limit":20},"writes":0,"reads":-1,"x":"abc","y":{"used":3}}}"#
        let c = try CoinPlanConfig.decode(data(json))
        XCTAssertEqual(c.pro.allowance, ["runs": 50, "recipients": 20], "数でない値・0 以下・limit の無い物は落とす")
        XCTAssertTrue(c.free.allowance.isEmpty, "枠は Pro の分だけ")
    }

    func testConfigAllowanceReadsNumericStringsAndMissingIsEmpty() throws {
        XCTAssertEqual(try CoinPlanConfig.decode(data(#"{"allowance":{"runs":"12"}}"#)).pro.allowance, ["runs": 12])
        XCTAssertEqual(try CoinPlanConfig.decode(data("{}")).pro.allowance, [:])
        XCTAssertEqual(try CoinPlanConfig.decode(data(#"{"allowance":[1,2]}"#)).pro.allowance, [:])
        XCTAssertEqual(try CoinPlanConfig.decode(data(#"{"allowance":null}"#)).pro.allowance, [:])
    }

    func testPlanNumbersAllowanceHelperDirectly() {
        XCTAssertEqual(CoinPlanNumbers.allowance(.object(["a": .number(1), "b": .number(1e300), "c": .number(.nan)])), ["a": 1],
                       "大きすぎる数・NaN は読まない")
        XCTAssertEqual(CoinPlanNumbers.allowance(nil), [:])
    }
}
#endif
