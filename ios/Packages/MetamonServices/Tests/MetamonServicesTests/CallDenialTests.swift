import XCTest
import MetamonDSL
@testable import MetamonServices

/// 通話の API の断り(CallDenial)と、通話の終わりの理由(CallSession.reason(for:))の分け方を表で確かめる。
@MainActor
final class CallDenialTests: XCTestCase {
    private func server(_ status: Int, _ code: String = "x", details: [String: JSONValue] = [:]) -> Error {
        APIError.server(status: status, code: code, message: "", details: details)
    }

    private struct Row {
        let name: String
        let error: Error
        let denial: CallDenial
        let reason: CallSession.EndReason
    }

    func testTable() {
        let coins: [String: JSONValue] = ["required": .number(30), "balance": .number(12)]
        let rows: [Row] = [
            Row(name: "402 コイン不足", error: server(402, "coins_insufficient", details: coins),
                denial: .coinsInsufficient(required: 30, balance: 12), reason: .coins(required: 30, balance: 12)),
            Row(name: "402 でも code が違えば other", error: server(402, "other_code", details: coins),
                denial: .other, reason: .failed),
            Row(name: "404 calls_disabled", error: server(404, details: ["reason": .string("calls_disabled")]),
                denial: .disabled, reason: .denied(.disabled)),
            Row(name: "404 call_ended", error: server(404, details: ["reason": .string("call_ended")]),
                denial: .ended, reason: .denied(.ended)),
            Row(name: "404 reason 無し", error: server(404), denial: .unavailable, reason: .denied(.unavailable)),
            Row(name: "404 知らない reason", error: server(404, details: ["reason": .string("zzz")]),
                denial: .unavailable, reason: .denied(.unavailable)),
            Row(name: "403 not_member", error: server(403, "not_member"), denial: .notMember, reason: .denied(.notMember)),
            Row(name: "429", error: server(429), denial: .rateLimited, reason: .denied(.rateLimited)),
            Row(name: "500", error: server(500), denial: .other, reason: .failed),
            Row(name: "401", error: server(401), denial: .other, reason: .failed),
            Row(name: "圏外(URLError)", error: URLError(.notConnectedToInternet), denial: .other, reason: .failed),
            Row(name: "APIError の http は 403 でも other", error: APIError.http(status: 403, body: "x"), denial: .other, reason: .failed),
        ]
        for row in rows {
            XCTAssertEqual(CallDenial(row.error), row.denial, row.name)
            XCTAssertEqual(CallSession.reason(for: row.error), row.reason, row.name)
        }
    }

    func testCoinsMissingDetailsDefaultToZero() {
        XCTAssertEqual(CallDenial(server(402, "coins_insufficient")), .coinsInsufficient(required: 0, balance: 0))
    }

    func testCoinsFractionTruncates() {
        let d: [String: JSONValue] = ["required": .number(10.9), "balance": .number(0.4)]
        XCTAssertEqual(CallDenial(server(402, "coins_insufficient", details: d)), .coinsInsufficient(required: 10, balance: 0))
    }

    func testCancellationIsOther() {
        XCTAssertEqual(CallDenial(CancellationError()), .other)
    }
}
