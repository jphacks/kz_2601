import XCTest
import MetamonDSL
import MetamonRenderer
import MetamonServices
@testable import Metamon

/// #2001: 外の API の 402・409・503 を「次にすること」(つなぐ・Pro・コイン・トースト)に振り分ける。
@MainActor
final class ExtDenialTests: XCTestCase {
    private var saved: AppLanguage = .system

    override func setUp() {
        saved = LanguagePreference.shared.setting
        LanguagePreference.shared.setting = .ja
    }

    override func tearDown() { LanguagePreference.shared.setting = saved }

    private func server(_ status: Int, _ code: String, _ details: [String: JSONValue] = [:]) -> APIError {
        .server(status: status, code: code, message: "", details: details)
    }

    // MARK: - ExtDenial(コードごと)

    func testNotConnectedWithProvider() {
        XCTAssertEqual(ExtDenial(server(409, "ext_not_connected", ["provider": .string("googleCalendar")])),
                       .notConnected(provider: "googleCalendar"))
    }

    func testNotConnectedWithoutProvider() {
        XCTAssertEqual(ExtDenial(server(409, "ext_not_connected")), .notConnected(provider: nil))
    }

    func testProRequiredUsesProRequired() {
        let e = server(402, "pro_required", ["features": .array([.object(["id": .string("externalServices"), "title": .string("つないだサービス")])])])
        XCTAssertEqual(ExtDenial(e), .proRequired(ProRequired(e)!))
        let creator = server(402, "pro_required", ["reason": .string("creator_needs_pro")])
        XCTAssertEqual(ExtDenial(creator), .proRequired(ProRequired(features: [], creatorNeedsPro: true)))
    }

    #if !APP_STORE_REVIEW
    func testCoinsUseCoinDenial() {
        let e = server(402, "coins_insufficient", ["required": .number(5), "balance": .number(1), "payer": .string("self")])
        XCTAssertEqual(ExtDenial(e), .coins(CoinDenial(kind: .coinsInsufficient, required: 5, balance: 1, payer: "self")))
        XCTAssertEqual(ExtDenial(server(402, "allowance_exhausted", ["payer": .string("creator")])),
                       .coins(CoinDenial(kind: .allowanceExhausted, payer: "creator")))
        XCTAssertEqual(ExtDenial(server(402, "overflow_cap_reached")), .coins(CoinDenial(kind: .overflowCapReached)))
    }
    #endif

    func testUnavailableAndBusy() {
        XCTAssertEqual(ExtDenial(server(503, "ext_unavailable")), .unavailable)
        XCTAssertEqual(ExtDenial(server(503, "ext_busy")), .busy)
    }

    /// 外の API の断りでないものは nil(呼び出し側のふつうのエラーに任せる)
    func testOtherErrorsAreNil() {
        XCTAssertNil(ExtDenial(server(409, "duplicate_request")))
        XCTAssertNil(ExtDenial(server(402, "something_new")))
        XCTAssertNil(ExtDenial(server(500, "internal")))
        XCTAssertNil(ExtDenial(APIError.http(status: 502, body: "")))
        XCTAssertNil(ExtDenial(URLError(.notConnectedToInternet)))
    }

    #if !APP_STORE_REVIEW
    // MARK: - 行き先

    func testRouteNotConnected() {
        XCTAssertEqual(ExtDenialRoute.of(.notConnected(provider: "todoist"), isPro: true), .connect(provider: "todoist"))
        // 知らない provider もつなぐ案内(名前は id のまま)
        XCTAssertEqual(ExtDenialRoute.of(.notConnected(provider: "someNew"), isPro: false), .connect(provider: "someNew"))
        XCTAssertEqual(ConnectedServicesState.name(nil, id: "someNew", english: false), "someNew")
        // provider が分からない → 設定へ向けるトースト
        XCTAssertEqual(ExtDenialRoute.of(.notConnected(provider: nil), isPro: true),
                       .toast("外のサービスとつながっていません。設定の「つないだサービス」からつないでください。"))
    }

    func testProviderNamePrefersServer() {
        let p = ExtProvider(id: "fakeOauth", nameJa: "試験用のサービス", nameEn: "Test service")
        XCTAssertEqual(ConnectedServicesState.name(p, id: "fakeOauth", english: false), "試験用のサービス")
        XCTAssertEqual(ConnectedServicesState.name(p, id: "fakeOauth", english: true), "Test service")
        XCTAssertEqual(ConnectedServicesState.name(nil, id: "googleCalendar", english: false), "Google カレンダー", "古いサーバーは端末の名前")
        XCTAssertEqual(ExtDenialCopy.connectTitle("Todoist"), "Todoist とつないでください")
    }

    func testRouteProRequired() {
        let feature = PaidFeature(id: "externalServices", title: "x", reason: "y")
        XCTAssertEqual(ExtDenialRoute.of(.proRequired(ProRequired(features: [feature])), isPro: false), .paywall([feature]))
        XCTAssertEqual(ExtDenialRoute.of(.proRequired(ProRequired(features: [])), isPro: false),
                       .paywall([ConnectedServicesSection.paidFeature]), "features が無ければ「つないだサービス」")
        XCTAssertEqual(ExtDenialRoute.of(.proRequired(ProRequired(features: [], creatorNeedsPro: true)), isPro: false),
                       .toast("このアプリを作った人が Tailor It Pro ではないため、外のサービスを使えません"), "参加者は払っても直らない")
        XCTAssertEqual(ExtDenialRoute.of(.proRequired(ProRequired(features: [])), isPro: true),
                       .toast("購入の確認がまだサーバーに届いていません。少し待ってから、もう一度お試しください。"), "課金画面を繰り返さない")
    }

    func testRouteCoinsAndToasts() {
        let coins = CoinDenial(kind: .coinsInsufficient, required: 5, balance: 1)
        XCTAssertEqual(ExtDenialRoute.of(.coins(coins), isPro: true), .coins(coins))
        XCTAssertEqual(ExtDenialRoute.of(.unavailable, isPro: true), .toast("外のサービスはいま使えません。しばらくしてからお試しください。"))
        XCTAssertEqual(ExtDenialRoute.of(.busy, isPro: true), .toast("外のサービスが混み合っています。少し待ってから、もう一度お試しください。"))
    }

    func testEnglishCopy() {
        LanguagePreference.shared.setting = .en
        XCTAssertEqual(ExtDenialCopy.connectTitle("Todoist"), "Connect Todoist")
        XCTAssertEqual(ExtDenialCopy.busy, "The outside service is busy. Please wait a moment and try again.")
        XCTAssertEqual(ProRequired(features: []).extMessage, "Using outside services needs Tailor It Pro")
    }

    // MARK: - 「つなぐ」は設定と同じ流れ

    func testConnectEntersExistingFlow() async {
        let mock = MockExternalServicesClient()
        let model = ConnectedServicesModel()
        let task = ConnectFlow.start("fakeOauth", model: model, client: mock, isPro: true) { _ in
            XCTFail("モックは同意の画面を開かない")
            return URL(string: "tailorit://connected?status=ok")!
        }
        await task?.value
        let started = await mock.started
        XCTAssertEqual(started, ["fakeOauth"])
        guard case .loaded(let rows) = model.state else { return XCTFail("\(model.state)") }
        XCTAssertEqual(rows.first { $0.provider == "fakeOauth" }?.status, .connected)
    }

    func testConnectWithoutProDoesNothing() async {
        let mock = MockExternalServicesClient()
        XCTAssertNil(ConnectFlow.start("fakeOauth", model: ConnectedServicesModel(), client: mock, isPro: false) { $0 })
        let started = await mock.started
        XCTAssertEqual(started, [], "Pro でなければ課金画面(呼び出し側)")
    }

    #if DEBUG
    // MARK: - 開発用の呼び出し口

    func testDebugErrorsMapToDenials() throws {
        XCTAssertEqual(ExtDenial(try XCTUnwrap(DebugExtDenial.error(code: "ext_not_connected"))), .notConnected(provider: "fakeOauth"))
        XCTAssertEqual(ExtDenial(try XCTUnwrap(DebugExtDenial.error(code: "pro_required"))), .proRequired(ProRequired(features: [])))
        XCTAssertEqual(ExtDenial(try XCTUnwrap(DebugExtDenial.error(code: "coins_insufficient"))),
                       .coins(CoinDenial(kind: .coinsInsufficient, required: 5, balance: 1)))
        XCTAssertEqual(ExtDenial(try XCTUnwrap(DebugExtDenial.error(code: "ext_unavailable"))), .unavailable)
        XCTAssertEqual(ExtDenial(try XCTUnwrap(DebugExtDenial.error(code: "ext_busy"))), .busy)
        XCTAssertNil(DebugExtDenial.error(code: nil))
    }

    /// モックの `setNextError` で仕込んだ断りが、偽の provider への op で返る
    func testDebugCallThroughMock() async {
        let mock = MockExternalServicesClient()
        await mock.setNextError(DebugExtDenial.error(code: "ext_not_connected"))
        let (denial, text) = await DebugExtDenial.call(mock)
        XCTAssertEqual(denial, .notConnected(provider: "fakeOauth"))
        XCTAssertNil(text)
        let (none, ok) = await DebugExtDenial.call(mock)
        XCTAssertNil(none)
        XCTAssertNotNil(ok)
    }
    #endif
    #endif
    // MARK: - つなぐ案内の「つなぐ」

    func testConnectButtonWaitsForProviders() {
        XCTAssertEqual(ExtConnectAvailability.of(.loading, provider: "fakeOauth", providersKnown: false), .loading, "読む前は押せない")
        XCTAssertEqual(ExtConnectAvailability.of(.hidden, provider: "fakeOauth", providersKnown: false), .unavailable)
        XCTAssertEqual(ExtConnectAvailability.of(.failed("x"), provider: "fakeOauth", providersKnown: false), .available)
        let on = ConnectedServiceRow(provider: "fakeOauth", name: "試験用のサービス", status: .notConnected, enabled: true)
        let off = ConnectedServiceRow(provider: "fakeOauth", name: "試験用のサービス", status: .notConnected, enabled: false)
        XCTAssertEqual(ExtConnectAvailability.of(.loaded([on]), provider: "fakeOauth", providersKnown: true), .available)
        XCTAssertEqual(ExtConnectAvailability.of(.loaded([off]), provider: "fakeOauth", providersKnown: true), .unavailable)
        XCTAssertEqual(ExtConnectAvailability.of(.loaded([]), provider: "fakeOauth", providersKnown: false), .available, "古いサーバー(providers 無し)はつなげる扱い")
    }

    /// 新しいサーバーが providers からこの provider を外した = 今つなげない(一覧の enabled=false と同じ)
    func testConnectUnavailableWhenProvidersOmitIt() {
        let other = ConnectedServiceRow(provider: "todoist", name: "Todoist", status: .notConnected)
        XCTAssertEqual(ExtConnectAvailability.of(.loaded([other]), provider: "fakeOauth", providersKnown: true), .unavailable)
        XCTAssertEqual(ExtConnectAvailability.of(.loaded([]), provider: "fakeOauth", providersKnown: true), .unavailable)
    }

    @MainActor
    func testModelRemembersWhetherProvidersKnown() async {
        let model = ConnectedServicesModel()
        await model.load(MockExternalServicesClient())
        XCTAssertTrue(model.providersKnown, "モックは providers を返す")
    }

    // MARK: - つなぐ案内を閉じる時

    func testDismissOnlyOnRealConnect() {
        let p = "fakeOauth"
        let not = ConnectedServicesState.loaded([ConnectedServiceRow(provider: p, name: "x", status: .notConnected)])
        let again = ConnectedServicesState.loaded([ConnectedServiceRow(provider: p, name: "x", status: .needsReconnect)])
        let done = ConnectedServicesState.loaded([ConnectedServiceRow(provider: p, name: "x", status: .connected)])
        XCTAssertFalse(ExtConnectAvailability.didConnect(from: .loading, to: done, provider: p),
                       "最初の読み込みで「つないだ」でも閉じない(サーバーは ext_not_connected と言っている)")
        XCTAssertTrue(ExtConnectAvailability.didConnect(from: not, to: done, provider: p))
        XCTAssertTrue(ExtConnectAvailability.didConnect(from: again, to: done, provider: p))
        XCTAssertTrue(ExtConnectAvailability.didConnect(from: .loaded([]), to: done, provider: p), "古いサーバーはつなぐ前は行が無い")
        XCTAssertFalse(ExtConnectAvailability.didConnect(from: done, to: done, provider: p))
        XCTAssertFalse(ExtConnectAvailability.didConnect(from: not, to: not, provider: p))
        XCTAssertFalse(ExtConnectAvailability.didConnect(from: .failed("x"), to: done, provider: p))
    }
}
