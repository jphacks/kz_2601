import XCTest
import MetamonDSL
import MetamonServices
@testable import Metamon

/// 持てるアプリの数の上限(#630・402 app_limit_reached)と、置き換えシートの決め方(純粋関数)。
@MainActor
final class AppReplacePlanTests: XCTestCase {
    private let t0 = Date(timeIntervalSince1970: 1_800_000_000)

    private func limitError(apps: [JSONValue]? = nil, canBuy: Bool = false, slotCoins: Double = 0, balance: Double? = nil,
                            held: Double = 2, limit: Double = 2) -> APIError {
        var d: [String: JSONValue] = ["resource": .string("apps"), "plan": .string("free"), "held": .number(held),
                                      "limit": .number(limit), "base": .number(2), "purchasedSlots": .number(0),
                                      "canBuySlot": .bool(canBuy), "slotCoins": .number(slotCoins), "upgrade": .bool(true)]
        if let apps { d["apps"] = .array(apps) }
        if let balance { d["balance"] = .number(balance) }
        return .server(status: 402, code: "app_limit_reached", message: "持てるアプリの数(2 個)に達しました", details: d)
    }

    private func held(_ id: String, _ name: String, mine: Bool, updatedAt: String = "2026-11-01T00:00:00Z") -> JSONValue {
        .object(["id": .string(id), "name": .string(name), "mine": .bool(mine), "updatedAt": .string(updatedAt)])
    }

    // MARK: - decode

    func testDecodesAppLimit() throws {
        let error = limitError(apps: [held("a", "家計簿", mine: true), held("b", "家族の買い物", mine: false)],
                               canBuy: true, slotCoins: 7, balance: 3)
        let limit = try XCTUnwrap(AppLimit(error))
        XCTAssertEqual(limit.held, 2)
        XCTAssertEqual(limit.limit, 2)
        XCTAssertTrue(limit.canBuySlot)
        XCTAssertEqual(limit.slotCoins, 7)
        XCTAssertEqual(limit.balance, 3)
        XCTAssertTrue(limit.upgrade)
        XCTAssertEqual(limit.apps?.map(\.id), ["a", "b"])
        XCTAssertEqual(limit.apps?.map(\.mine), [true, false])
        XCTAssertNotNil(limit.apps?.first?.updatedAt)
        guard case .appLimit(let classified) = FailureKind.classify(error) else {
            return XCTFail("app_limit_reached は 402 rejected として扱わない")
        }
        XCTAssertEqual(classified, limit)
    }

    func testNotAppLimit() {
        XCTAssertNil(AppLimit(APIError.server(status: 402, code: "coins_insufficient", message: "")))
        XCTAssertNil(AppLimit(APIError.server(status: 403, code: "plan_limit_reached", message: "")))
        // 値段が無ければ買えるとは言わない
        XCTAssertEqual(AppLimit(limitError(canBuy: true, slotCoins: 0))?.canBuySlot, false)
    }

    func testCardOffersReplace() throws {
        let limit = try XCTUnwrap(AppLimit(limitError()))
        let card = try XCTUnwrap(ErrorPresentation.from(.appLimit(limit), transcript: "家計簿を作って"))
        XCTAssertEqual(card.kind, .appLimit(limit))
        XCTAssertEqual(card.primary, .replaceApp)
        XCTAssertEqual(card.transcript, "家計簿を作って")
    }

    // MARK: - plan

    func testRowsFromServerOldestFirstCurrentLast() throws {
        let limit = try XCTUnwrap(AppLimit(limitError(apps: [
            held("a", "家計簿", mine: true), held("b", "", mine: false), held("c", "日記", mine: true),
        ], held: 3, limit: 2)))
        let opened: [String: Date] = ["a": t0.addingTimeInterval(100), "b": t0]
        let local = [AppSummary(id: "b", name: "家族の買い物", currentVersion: 3)]
        let plan = AppReplacePlan.make(limit, local: local, lastOpened: { opened[$0] }, current: "a", coinsEnabled: true)
        // c は端末で開いた記録が無いのでサーバーの更新日時(古い)・使用中の a は最後
        XCTAssertEqual(plan.rows.map(\.id), ["c", "b", "a"])
        XCTAssertEqual(plan.rows.map(\.role), [.own, .joined, .own])
        XCTAssertEqual(plan.rows[1].name, "家族の買い物", "サーバーの名前が空なら端末の名前")
        XCTAssertTrue(plan.rows[2].isCurrent)
        XCTAssertEqual(plan.removalsNeeded, 2, "上限を下げた後は 2 つ消さないと入らない")
        XCTAssertNil(plan.slot, "サーバーが買えると言わなければ枠を出さない")
    }

    func testFallsBackToLocalList() throws {
        let limit = try XCTUnwrap(AppLimit(limitError()))
        let local = [
            AppSummary(id: "x", name: "記録", currentVersion: 4, updatedAt: "2026-11-02T00:00:00Z"),
            AppSummary(id: "blank", name: "はじめてのアプリ", currentVersion: 1),
            AppSummary(id: "old", name: "古い", currentVersion: 3, status: "archived"),
            AppSummary(id: "y", name: "日記", currentVersion: 2, updatedAt: "2026-11-01T00:00:00Z"),
        ]
        let plan = AppReplacePlan.make(limit, local: local, lastOpened: { _ in nil }, current: nil, coinsEnabled: false)
        XCTAssertEqual(plan.rows.map(\.id), ["y", "x"], "空のアプリ・アーカイブは数えないので出さない")
        XCTAssertEqual(Set(plan.rows.map(\.role)), [.own])
    }

    func testSlotOfferNeedsServerAndCoinFlag() throws {
        let limit = try XCTUnwrap(AppLimit(limitError(apps: [held("a", "A", mine: true)], canBuy: true, slotCoins: 7, balance: 3)))
        XCTAssertNil(AppReplacePlan.make(limit, local: [], lastOpened: { _ in nil }, current: nil, coinsEnabled: false).slot,
                     "コインの画面が無効なら出さない")
        let plan = AppReplacePlan.make(limit, local: [], lastOpened: { _ in nil }, current: nil, coinsEnabled: true)
        XCTAssertEqual(plan.slot, AppReplacePlan.SlotOffer(coins: 7, affordable: false))
        var rich = limit
        rich.balance = nil
        XCTAssertEqual(AppReplacePlan.make(rich, local: [], lastOpened: { _ in nil }, current: nil, coinsEnabled: true).slot?.affordable,
                       true, "残高が分からなければ押せる(足りなければお店)")
    }

    func testActionsAndContinuation() throws {
        let limit = try XCTUnwrap(AppLimit(limitError(apps: [
            held("a", "A", mine: true), held("b", "B", mine: false), held("c", "C", mine: true),
        ], held: 3, limit: 2)))
        let plan = AppReplacePlan.make(limit, local: [], lastOpened: { _ in nil }, current: nil, coinsEnabled: false)
        let own = try XCTUnwrap(plan.rows.first { $0.id == "a" })
        let joined = try XCTUnwrap(plan.rows.first { $0.id == "b" })
        XCTAssertEqual(AppReplacePlan.action(for: own), .delete(appId: "a"))
        XCTAssertEqual(AppReplacePlan.action(for: joined), .leave(appId: "b"))

        let afterOne = try XCTUnwrap(plan.removing("a"), "3 個で上限 2 なら 1 つ消しても続けられない")
        XCTAssertEqual(afterOne.held, 2)
        XCTAssertEqual(afterOne.rows.map(\.id).sorted(), ["b", "c"])
        XCTAssertNil(afterOne.removing("b"), "もう 1 つ消せば入る")
        XCTAssertEqual(plan.removing("zzz"), plan, "知らない行は何も変えない")
        XCTAssertNil(afterOne.addingSlot(), "枠を 1 つ買えば入る")
    }

    #if !APP_STORE_REVIEW // 審査用ビルドはコインを持たない(#450)
    func testCoinModelsReadSlots() throws {
        let wallet = try CoinWallet.decode(Data(#"{"mode":"enforce","purchased":5,"appSlots":2}"#.utf8))
        XCTAssertEqual(wallet.appSlots, 2)
        let bought = try CoinSlotPurchase.decode(Data(#"{"bought":true,"appSlots":3,"coins":7,"wallet":{"purchased":1,"appSlots":3}}"#.utf8))
        XCTAssertEqual(bought, CoinSlotPurchase(bought: true, appSlots: 3, coins: 7,
                                                wallet: CoinWallet(balance: CoinBalance(purchased: 1), appSlots: 3)))
        let config = try CoinPlanConfig.decode(Data(#"{"mode":"enforce","appSlotCoins":7}"#.utf8))
        XCTAssertEqual(config.appSlotCoins, 7)
        XCTAssertNil(try CoinPlanConfig.decode(Data(#"{"mode":"enforce"}"#.utf8)).appSlotCoins)
    }
    #endif
}
