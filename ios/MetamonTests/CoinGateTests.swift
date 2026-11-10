#if !APP_STORE_REVIEW // 審査用ビルドはコインを持たない(#450)
import XCTest
import MetamonServices
@testable import Metamon

/// #630: コインの断りの行き先・フラグ・文言。
@MainActor
final class CoinGateTests: XCTestCase {
    func testDenialRoute() {
        XCTAssertEqual(CoinDenialRoute.of(CoinDenial(kind: .coinsInsufficient, payer: "self")), .shop)
        XCTAssertEqual(CoinDenialRoute.of(CoinDenial(kind: .coinsInsufficient)), .shop, "payer が無ければ自分")
        XCTAssertEqual(CoinDenialRoute.of(CoinDenial(kind: .allowanceExhausted)), .settings)
        XCTAssertEqual(CoinDenialRoute.of(CoinDenial(kind: .overflowCapReached)), .settings)
        XCTAssertEqual(CoinDenialRoute.of(CoinDenial(kind: .coinsInsufficient, payer: "creator")), .explainOnly,
                       "作成者が払うアプリでは参加者にお店を出さない")
    }

    func testDisabledGateShowsNothing() async {
        let gate = CoinGate(store: CoinStore(mode: .mock, service: MockCoinService(config: .disabled)))
        await gate.store.refreshConfig()
        gate.presenterAppeared(UUID())
        XCTAssertFalse(gate.isEnabled)
        XCTAssertFalse(gate.handle(CoinDenial(kind: .coinsInsufficient)), "フラグが OFF なら引き受けない(今のエラー表示のまま)")
        gate.openShop()
        XCTAssertNil(gate.request)
    }

    func testEnabledGatePresentsOnTopmost() async {
        let gate = CoinGate(store: CoinStore(mode: .mock, service: MockCoinService()))
        await gate.store.refreshConfig()
        XCTAssertFalse(gate.handle(CoinDenial(kind: .coinsInsufficient)), "出す場所が無ければ引き受けない")
        let root = UUID(), settings = UUID()
        gate.presenterAppeared(root)
        gate.presenterAppeared(settings)
        XCTAssertTrue(gate.handle(CoinDenial(kind: .coinsInsufficient, required: 30, balance: 5)))
        XCTAssertEqual(gate.request?.presenter, settings, "いちばん手前のシートから出す")
        XCTAssertEqual(gate.request?.sheet, .shop(CoinDenial(kind: .coinsInsufficient, required: 30, balance: 5)))
        gate.presenterDisappeared(settings)
        XCTAssertNil(gate.request, "出していたシートが消えたら要求も消す")
        XCTAssertTrue(gate.handle(CoinDenial(kind: .allowanceExhausted)))
        XCTAssertEqual(gate.request?.presenter, root)
        XCTAssertEqual(gate.request?.sheet, .settings(CoinDenial(kind: .allowanceExhausted)))
    }

    func testCopyUsesServerNumbers() {
        XCTAssertTrue(CoinCopy.denialBody(CoinDenial(kind: .coinsInsufficient, required: 30, balance: 5)).contains("30"))
        XCTAssertEqual(CoinCopy.orderedAllowanceKeys(["writes", "unknown", "runs"]), ["runs", "writes"], "知らない枠は出さない")
        XCTAssertNil(CoinCopy.allowanceName("unknown"))
        XCTAssertEqual(CoinCopy.signedDelta(30), "+30")
        XCTAssertEqual(CoinCopy.signedDelta(-10), "-10")
        XCTAssertEqual(CoinCopy.operationName("ai_button"), CoinCopy.operationName("aiButton"))
        // サーバーの名前(backend/internal/coins)
        XCTAssertEqual(CoinCopy.orderedAllowanceKeys(["reads", "writes", "recipients", "runs"]), ["runs", "recipients", "writes", "reads"])
        XCTAssertEqual(CoinCopy.operationName("generate"), CoinCopy.operationName("create"))
        XCTAssertEqual(CoinCopy.operationName("text"), CoinCopy.operationName("aiButton"))
        for kind in ["grant_initial", "grant_monthly", "grant_expire", "purchase", "spend_ai", "spend_overflow", "refund_failed", "refund_apple", "adjust"] {
            XCTAssertNotEqual(CoinCopy.ledgerKind(kind), kind, "\(kind) に名前がある")
        }
    }
}
#endif
