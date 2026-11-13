import XCTest
import MetamonRenderer
import MetamonServices
@testable import Metamon

#if !APP_STORE_REVIEW
/// 外の API の断りの「次にすること」(#3078): iOS と共有する判定(`ExtDenialRoute`)を Mac のターゲットで確かめる
@MainActor
final class MacExtDenialTests: XCTestCase {
    private var saved: AppLanguage = .system

    override func setUp() {
        saved = LanguagePreference.shared.setting
        LanguagePreference.shared.setting = .ja
    }

    override func tearDown() { LanguagePreference.shared.setting = saved }

    func testNotConnectedGoesToConnect() {
        XCTAssertEqual(ExtDenialRoute.of(.notConnected(provider: "todoist"), isPro: true), .connect(provider: "todoist"))
        XCTAssertEqual(ExtDenialRoute.of(.notConnected(provider: nil), isPro: true),
                       .toast("外のサービスとつながっていません。設定の「つないだサービス」からつないでください。"))
    }

    func testProRequiredGoesToPaywallUnlessUseless() {
        let feature = PaidFeature(id: "externalServices", title: "x", reason: "y")
        XCTAssertEqual(ExtDenialRoute.of(.proRequired(ProRequired(features: [feature])), isPro: false), .paywall([feature]))
        XCTAssertEqual(ExtDenialRoute.of(.proRequired(ProRequired(features: [])), isPro: false),
                       .paywall([ExtDenialCopy.externalServicesFeature]), "features が無ければ「つないだサービス」")
        XCTAssertEqual(ExtDenialRoute.of(.proRequired(ProRequired(features: [], creatorNeedsPro: true)), isPro: false),
                       .toast("このアプリを作った人が Tailor It Pro ではないため、外のサービスを使えません"))
        XCTAssertEqual(ExtDenialRoute.of(.proRequired(ProRequired(features: [])), isPro: true),
                       .toast("購入の確認がまだサーバーに届いていません。少し待ってから、もう一度お試しください。"))
    }

    func testCoinsAndWaitingToasts() {
        let coins = CoinDenial(kind: .coinsInsufficient, required: 5, balance: 1)
        XCTAssertEqual(ExtDenialRoute.of(.coins(coins), isPro: true), .coins(coins))
        XCTAssertFalse(CoinCopy.denialBody(coins).isEmpty, "Mac はコインの画面が無いので、この説明がトーストになる")
        XCTAssertEqual(ExtDenialRoute.of(.unavailable, isPro: true), .toast(ExtDenialCopy.unavailable))
        XCTAssertEqual(ExtDenialRoute.of(.busy, isPro: true), .toast(ExtDenialCopy.busy))
    }

    func testExternalServicesFeatureIsSharedWithSettings() {
        XCTAssertEqual(MacConnectedServicesSection.paidFeature, ExtDenialCopy.externalServicesFeature)
    }

    func testConnectAvailability() {
        let on = ConnectedServiceRow(provider: "fakeOauth", name: "試験用", status: .notConnected, enabled: true)
        let off = ConnectedServiceRow(provider: "fakeOauth", name: "試験用", status: .notConnected, enabled: false)
        XCTAssertEqual(ExtConnectAvailability.of(.loading, provider: "fakeOauth", providersKnown: true), .loading)
        XCTAssertEqual(ExtConnectAvailability.of(.hidden, provider: "fakeOauth", providersKnown: true), .unavailable)
        XCTAssertEqual(ExtConnectAvailability.of(.failed("x"), provider: "fakeOauth", providersKnown: true), .available)
        XCTAssertEqual(ExtConnectAvailability.of(.loaded([on]), provider: "fakeOauth", providersKnown: true), .available)
        XCTAssertEqual(ExtConnectAvailability.of(.loaded([off]), provider: "fakeOauth", providersKnown: true), .unavailable)
        XCTAssertEqual(ExtConnectAvailability.of(.loaded([]), provider: "fakeOauth", providersKnown: true), .unavailable)
        XCTAssertEqual(ExtConnectAvailability.of(.loaded([]), provider: "fakeOauth", providersKnown: false), .available)
    }

    func testDidConnectOnlyAfterLoadedNotConnectedToConnected() {
        let before = ConnectedServiceRow(provider: "todoist", name: "Todoist", status: .notConnected)
        let after = ConnectedServiceRow(provider: "todoist", name: "Todoist", status: .connected)
        XCTAssertTrue(ExtConnectAvailability.didConnect(from: .loaded([before]), to: .loaded([after]), provider: "todoist"))
        XCTAssertFalse(ExtConnectAvailability.didConnect(from: .loading, to: .loaded([after]), provider: "todoist"))
        XCTAssertFalse(ExtConnectAvailability.didConnect(from: .loaded([before]), to: .loaded([before]), provider: "todoist"))
    }
}
#endif
