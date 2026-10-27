import XCTest
import StoreKitTest
import MetamonServices
@testable import Metamon

/// 課金(#77)をアプリのプロセス内で確認する。SKTestSession はホストアプリ(Bundle ID)が無いと商品を返さないため、ここに置く。
/// 構成ファイルはアプリの Scheme と同じ `ios/StoreKit/TailorItPro.storekit` をテストバンドルのリソースとして読む(コピーを持たない)。
@MainActor
final class PurchaseFlowTests: XCTestCase {
    private var session: SKTestSession!

    /// xcodebuild(CLI)からの実行では SKTestSession が `SKInternalErrorDomain Code=3` で構成を保存できず、商品が空になる(Xcode 26.5 で実測)。
    /// その時は購入部分だけ skip する(無料枠の判定は検証する)。Xcode の Test(⌘U)では購入まで通る想定。
    static let headlessNote = "StoreKit のテスト構成が読めない環境(xcodebuild CLI 等)。Xcode の ⌘U で実行してください"

    override func setUp() async throws {
        session = try SKTestSession(configurationFileNamed: "TailorItPro")
        session.resetToDefaultState()
        session.clearTransactions()
        session.disableDialogs = true
    }

    override func tearDown() async throws {
        session?.clearTransactions()
        session = nil
    }

    func testPurchaseGrantsPro() async throws {
        let store = ProEntitlementStore()
        await store.loadProducts()
        try XCTSkipIf(store.products.isEmpty, Self.headlessNote)
        XCTAssertEqual(store.monthly?.id, ProProducts.monthly)
        XCTAssertNotNil(store.monthly?.displayPrice)
        await store.refreshEntitlements()
        XCTAssertFalse(store.isPro)

        let outcome = try await store.purchase(productID: ProProducts.monthly)
        XCTAssertEqual(outcome, .purchased)
        XCTAssertTrue(store.isPro)
        XCTAssertEqual(store.activeProductID, ProProducts.monthly)
        XCTAssertNotNil(store.expiresAt)

        // 取引を消すと権利が外れる(返金・期限切れ相当)
        session.clearTransactions()
        await store.refreshEntitlements()
        XCTAssertFalse(store.isPro)
    }

    /// 無料枠 1 回 → 2 回目は上限シート → 購入 → 無制限。
    func testGateBlocksAfterFreeLimitThenPurchaseUnlocks() async throws {
        let defaults = UserDefaults(suiteName: "PurchaseFlowTests.\(UUID().uuidString)")!
        let gate = ProGate(store: ProEntitlementStore(), usage: CreationUsageStore(defaults: defaults),
                           quota: CreationQuota(dailyFreeLimit: 1), limitDisabled: false)
        XCTAssertTrue(gate.admitNewApp())
        gate.recordNewApp()
        XCTAssertEqual(gate.decision, .blocked)
        XCTAssertFalse(gate.admitNewApp())
        try await Task.sleep(for: .milliseconds(700))
        XCTAssertTrue(gate.paywallPresented)
        XCTAssertEqual(gate.paywall?.reason, .limitReached)
        gate.finishPaywall()

        await gate.store.loadProducts()
        try XCTSkipIf(gate.store.products.isEmpty, Self.headlessNote)
        _ = try await gate.store.purchase(productID: ProProducts.monthly)
        XCTAssertTrue(gate.isPro)
        XCTAssertEqual(gate.decision, .unlimited)
        XCTAssertTrue(gate.admitNewApp())
    }

    /// デモ端末(制限の無効化)は回数に関係なく通す。
    func testLimitDisabledNeverBlocks() {
        let defaults = UserDefaults(suiteName: "PurchaseFlowTests.\(UUID().uuidString)")!
        let gate = ProGate(store: ProEntitlementStore(), usage: CreationUsageStore(defaults: defaults),
                           quota: CreationQuota(dailyFreeLimit: 0), limitDisabled: true)
        XCTAssertTrue(gate.admitNewApp())
        XCTAssertFalse(gate.paywallPresented)
    }
}
