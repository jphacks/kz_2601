#if !APP_STORE_REVIEW
import XCTest
import MetamonServices
@testable import Metamon

/// 課金画面の出し方(#2852): 理由ごとの見出し・Pro の時は出さない・同時に 2 つ出さない。
/// どのウインドウに出すかは MacPaywallRoutingTests。
@MainActor
final class MacPaywallPresenterTests: XCTestCase {
    private func pro(front: [Int] = [1], presenters: [Int] = [1]) -> MacPro {
        let pro = MacPro(store: ProEntitlementStore(mode: .mock))
        pro.frontWindows = { front }
        presenters.forEach { pro.registerPresenter(window: $0) }
        return pro
    }

    // MARK: - 理由ごとの文

    func testHeadlineDiffersByReason() {
        let settings = MacPaywallView.headline(.settings)
        let limit = MacPaywallView.headline(.planLimit)
        let feature = MacPaywallView.headline(.paidFeature([PaidFeature(id: "sharedData", title: "掲示板")]))
        XCTAssertEqual(Set([settings, limit, feature]).count, 3)
        XCTAssertFalse(settings.isEmpty)
        XCTAssertFalse(limit.isEmpty)
        XCTAssertTrue(feature.contains("掲示板"), "求めた機能の名前を見出しに出す")
    }

    func testFeatureNamesJoinTitles() {
        let names = MacPaywallView.featureNames([
            PaidFeature(id: "a", title: "掲示板"),
            PaidFeature(id: "b", title: ""),
            PaidFeature(id: "c", title: "ランキング"),
        ])
        XCTAssertEqual(names, "掲示板" + L10n.tr("・") + "ランキング", "空の名前は飛ばす")
    }

    func testFeatureNamesFallback() {
        let fallback = L10n.tr("みんなで共有するデータ")
        XCTAssertEqual(MacPaywallView.featureNames([]), fallback)
        XCTAssertEqual(MacPaywallView.featureNames([PaidFeature(id: "x", title: "")]), fallback)
        XCTAssertTrue(MacPaywallView.headline(.paidFeature([])).contains(fallback))
    }

    func testRequestKeepsReasonAndCloseNote() {
        let pro = pro()
        pro.requestPro(.planLimit, closeNote: "作れるのは 3 つまでです")
        XCTAssertEqual(pro.paywall?.reason, .planLimit)
        XCTAssertEqual(pro.paywall?.closeNote, "作れるのは 3 つまでです")
        XCTAssertEqual(pro.paywall(for: 1)?.reason, .planLimit)
    }

    // MARK: - Pro の時は出さない

    func testDoesNotRequestWhenPro() async throws {
        let pro = pro()
        let outcome = try await pro.store.purchase(productID: ProProducts.monthly)
        XCTAssertEqual(outcome, .purchased)
        XCTAssertTrue(pro.isPro)
        pro.requestPro(.planLimit)
        pro.requestPro(.paidFeature([]))
        XCTAssertNil(pro.paywall)
        XCTAssertNil(pro.paywall(for: 1))
    }

    // MARK: - 同時に 2 つ出さない

    func testSecondRequestReplacesFirst() {
        let pro = pro(front: [2, 1], presenters: [1, 2])
        pro.requestPro(.planLimit)
        let first = pro.paywall?.id
        pro.requestPro(.paidFeature([]))
        XCTAssertNotEqual(pro.paywall?.id, first, "新しい要求に置き換える(積まない)")
        XCTAssertEqual(pro.paywall?.reason, .paidFeature([]))
        XCTAssertNotNil(pro.paywall(for: 2))
        XCTAssertNil(pro.paywall(for: 1), "出すのは 1 つのウインドウだけ")
    }

    func testSameWindowRegisteredTwiceIsOnePresenter() {
        let pro = pro(front: [1], presenters: [1, 1])
        XCTAssertEqual(pro.presenterWindows, [1])
        pro.requestPro(.planLimit)
        pro.unregisterPresenter(window: 1)
        XCTAssertNil(pro.paywall, "1 回外せば消える")
    }

    func testDismissClears() {
        let pro = pro()
        pro.requestPro(.settings)
        XCTAssertNotNil(pro.paywall(for: 1))
        pro.dismissPaywall()
        XCTAssertNil(pro.paywall)
        XCTAssertNil(pro.paywall(for: 1))
    }
}
#endif
