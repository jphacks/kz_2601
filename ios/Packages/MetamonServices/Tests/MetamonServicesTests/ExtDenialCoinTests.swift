import XCTest
import MetamonDSL
@testable import MetamonServices

/// 外の API の断りのうち 402 のコインの道(`ExtDenial.init?` → private の `coinDenial`)。
/// 審査用の版(APP_STORE_REVIEW)は `.coins` を持たないので、ふつうの版だけで回す。
#if !APP_STORE_REVIEW
final class ExtDenialCoinTests: XCTestCase {
    private func server(_ status: Int, _ code: String, _ details: [String: JSONValue] = [:]) -> APIError {
        .server(status: status, code: code, message: "", details: details)
    }

    func testCoinCodesBecomeCoinDenial() {
        let details: [String: JSONValue] = ["required": .number(30), "balance": .number(12), "payer": .string("self")]
        let cases: [(code: String, kind: CoinDenial.Kind)] = [
            ("coins_insufficient", .coinsInsufficient),
            ("allowance_exhausted", .allowanceExhausted),
            ("overflow_cap_reached", .overflowCapReached),
        ]
        for c in cases {
            XCTAssertEqual(ExtDenial(server(402, c.code, details)),
                           .coins(CoinDenial(kind: c.kind, required: 30, balance: 12, payer: "self")), c.code)
        }
    }

    func testCoinDetailsAreOptionalAndLenient() {
        XCTAssertEqual(ExtDenial(server(402, "coins_insufficient")), .coins(CoinDenial(kind: .coinsInsufficient)),
                       "details が無くてもコインの断り")
        let creator: [String: JSONValue] = ["required": .string("5"), "balance": .bool(true), "payer": .string("creator")]
        guard case .coins(let d)? = ExtDenial(server(402, "allowance_exhausted", creator)) else {
            return XCTFail("コインの断りになる")
        }
        XCTAssertEqual(d.required, 5, "文字列の数も読む")
        XCTAssertNil(d.balance, "数でない値は nil")
        XCTAssertFalse(d.payerIsSelf, "作成者が払う")
    }

    /// 402 でもコインでない code・コインの code でも 402 でない・本文だけの 402 はコインにしない。
    func testNonCoinErrorsAreNotCoins() {
        let cases: [(name: String, error: Error, want: ExtDenial?)] = [
            ("402 の知らない code", server(402, "payment_required"), nil),
            ("402 の空の code", server(402, ""), nil),
            ("コインの code の 403", server(403, "coins_insufficient"), nil),
            ("コインの code の 409", server(409, "overflow_cap_reached"), nil),
            ("本文だけの 402", APIError.http(status: 402, body: #"{"error":{"code":"coins_insufficient"}}"#), nil),
            ("URL が不正", APIError.invalidURL, nil),
            ("APIError でない", URLError(.notConnectedToInternet), nil),
            ("402 の Pro は Pro の断り", server(402, "pro_required"), .proRequired(ProRequired(features: []))),
        ]
        for c in cases {
            XCTAssertEqual(ExtDenial(c.error), c.want, c.name)
        }
    }

    /// 外の API の他の断りはコインより先に分ける(同じ init の順)。
    func testOtherExtDenialsStillWin() {
        XCTAssertEqual(ExtDenial(server(409, "ext_not_connected", ["provider": .string("google")])), .notConnected(provider: "google"))
        XCTAssertEqual(ExtDenial(server(503, "ext_unavailable")), .unavailable)
        XCTAssertEqual(ExtDenial(server(503, "ext_busy")), .busy)
    }
}
#endif
