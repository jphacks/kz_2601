import XCTest
@testable import MetamonServices

/// #414: 有料機能の入口で課金画面を出すかの判定(DSL を見ない分)と、モックのストア。
/// DSL の scope を見る判定(`PaidGate.introducesSharedData`)は同梱例を読むのでアプリをホストにした `MetamonTests/PaidGateFlowTests.swift`。
final class PaidGateDecisionTests: XCTestCase {
    func testNeedsPaywallOnlyWhenEnforcedAndNotPro() {
        XCTAssertTrue(PaidGate.needsPaywall(isPro: false, enforced: true))
        XCTAssertFalse(PaidGate.needsPaywall(isPro: true, enforced: true))
        XCTAssertFalse(PaidGate.needsPaywall(isPro: false, enforced: false), "デモ端末(課金の制限が無効)は出さない")
    }

    func testDropInstructionAsksForPrivate() {
        XCTAssertTrue(PaidGate.dropSharedDataInstruction(english: false).contains("自分だけ"))
        XCTAssertTrue(PaidGate.dropSharedDataInstruction(english: true).contains("private"))
    }
}

#if !APP_STORE_REVIEW // 審査用ビルドにはストアが無い(#450 G1)
/// モックのストア(UI テスト・`simctl launch` のシミュレータ)。App Store に触れずに購入 → Pro。
@MainActor
final class MockProStoreTests: XCTestCase {
    func testMockOfferPurchaseAndRestore() async throws {
        let store = ProEntitlementStore(mode: .mock)
        XCTAssertTrue(store.isMock)
        XCTAssertEqual(store.offer, .mockMonthly)
        XCTAssertEqual(store.offer?.displayPrice, "¥500")
        await store.refreshEntitlements()
        XCTAssertFalse(store.isPro)
        try await store.restore()
        XCTAssertFalse(store.isPro, "買っていなければ復元しても Pro にならない")

        let outcome = try await store.purchaseOffer()
        XCTAssertEqual(outcome, .purchased)
        XCTAssertTrue(store.isPro)
        XCTAssertEqual(store.activeProductID, ProProducts.monthly)
        XCTAssertNotNil(store.expiresAt)
        await store.refreshEntitlements()
        XCTAssertTrue(store.isPro, "権利の読み直しで外れない")
    }

    func testMockRejectsUnknownProduct() async {
        let store = ProEntitlementStore(mode: .mock)
        do {
            _ = try await store.purchase(productID: "unknown")
            XCTFail("知らない商品は買えない")
        } catch {
            XCTAssertEqual(error as? ProEntitlementStore.StoreError, .productNotFound)
        }
    }

    func testStoreKitModeIsNotMock() {
        XCTAssertFalse(ProEntitlementStore().isMock)
        XCTAssertFalse(ProEntitlementStore(mode: .storeKitOrMock).isMock, "商品が取れないと分かるまでは StoreKit")
    }
}
#endif
