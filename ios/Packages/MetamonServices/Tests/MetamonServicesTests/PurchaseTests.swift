// 審査用ビルド(APP_STORE_REVIEW・#450 G1)には課金が無い。
#if !APP_STORE_REVIEW
import XCTest
@testable import MetamonServices

/// 課金(#77): 無料枠の判定・日付の切り替わり・権利の判定。
final class CreationQuotaTests: XCTestCase {
    private var calendar: Calendar {
        var c = Calendar(identifier: .gregorian)
        c.timeZone = TimeZone(identifier: "Asia/Tokyo")!
        return c
    }

    private func date(_ s: String) -> Date {
        let f = ISO8601DateFormatter()
        return f.date(from: s)!
    }

    func testDecideFreeTierCountsDown() {
        let q = CreationQuota(dailyFreeLimit: 3)
        XCTAssertEqual(q.decide(used: 0, isPro: false, limitDisabled: false), .allowed(remaining: 3))
        XCTAssertEqual(q.decide(used: 2, isPro: false, limitDisabled: false), .allowed(remaining: 1))
        XCTAssertEqual(q.decide(used: 3, isPro: false, limitDisabled: false), .blocked)
        XCTAssertEqual(q.decide(used: 9, isPro: false, limitDisabled: false), .blocked)
    }

    func testProAndDisabledAreUnlimited() {
        let q = CreationQuota(dailyFreeLimit: 3)
        XCTAssertEqual(q.decide(used: 99, isPro: true, limitDisabled: false), .unlimited)
        XCTAssertEqual(q.decide(used: 99, isPro: false, limitDisabled: true), .unlimited)
    }

    func testZeroOrNegativeLimitBlocksImmediately() {
        XCTAssertEqual(CreationQuota(dailyFreeLimit: 0).decide(used: 0, isPro: false, limitDisabled: false), .blocked)
        XCTAssertEqual(CreationQuota(dailyFreeLimit: -2).dailyFreeLimit, 0)
    }

    func testLedgerResetsOnNewLocalDay() {
        // JST 23:59 と翌 00:01 は別の日
        let late = date("2026-10-24T14:59:00Z")
        let nextDay = date("2026-10-24T15:01:00Z")
        var ledger = CreationLedger()
        ledger = ledger.recording(on: late, calendar: calendar)
        ledger = ledger.recording(on: late, calendar: calendar)
        XCTAssertEqual(ledger.day, "2026-10-24")
        XCTAssertEqual(ledger.used(on: late, calendar: calendar), 2)
        XCTAssertEqual(ledger.used(on: nextDay, calendar: calendar), 0)
        XCTAssertEqual(ledger.recording(on: nextDay, calendar: calendar), CreationLedger(day: "2026-10-25", count: 1))
    }

    func testUsageStorePersists() {
        let defaults = UserDefaults(suiteName: "CreationQuotaTests.\(UUID().uuidString)")!
        let store = CreationUsageStore(defaults: defaults)
        let now = date("2026-10-24T03:00:00Z")
        XCTAssertEqual(store.used(on: now, calendar: calendar), 0)
        store.record(on: now, calendar: calendar)
        store.record(on: now, calendar: calendar)
        XCTAssertEqual(CreationUsageStore(defaults: defaults).used(on: now, calendar: calendar), 2)
        store.reset()
        XCTAssertEqual(store.used(on: now, calendar: calendar), 0)
    }

    func testEntitlementActive() {
        let now = date("2026-10-24T03:00:00Z")
        let id = ProProducts.monthly
        XCTAssertTrue(ProEntitlement.isActive(productID: id, revocationDate: nil, expirationDate: now.addingTimeInterval(60), now: now))
        XCTAssertFalse(ProEntitlement.isActive(productID: id, revocationDate: nil, expirationDate: now.addingTimeInterval(-60), now: now))
        XCTAssertFalse(ProEntitlement.isActive(productID: id, revocationDate: now, expirationDate: nil, now: now))
        XCTAssertFalse(ProEntitlement.isActive(productID: "other", revocationDate: nil, expirationDate: nil, now: now))
        XCTAssertTrue(ProEntitlement.isActive(productID: id, revocationDate: nil, expirationDate: nil, now: now))
    }
}

// StoreKitTest(SKTestSession)の購入テストはここに置かない: パッケージのテストはホストアプリ(Bundle ID)が無く、
// SKTestSession を開いても Product.products が空で返る(実測)。購入 → 権利の確認はアプリをホストにした `ios/MetamonTests/PurchaseFlowTests.swift` で行う。
#endif
