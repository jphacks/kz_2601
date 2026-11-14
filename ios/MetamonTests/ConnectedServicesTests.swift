import AuthenticationServices
import UIKit
import XCTest
import MetamonDSL
import MetamonRenderer
import MetamonServices
@testable import Metamon

/// #1493: 設定の「つないだサービス」・コインの履歴の `spend_ext`。
@MainActor
final class ConnectedServicesTests: XCTestCase {
    private var saved: AppLanguage = .system

    override func setUp() {
        saved = LanguagePreference.shared.setting
        LanguagePreference.shared.setting = .ja
    }

    override func tearDown() { LanguagePreference.shared.setting = saved }

    /// 審査用の版は OAuth の provider を外す(決定 6)ので、設定に出さない。
    func testHiddenInReviewBuild() {
        #if APP_STORE_REVIEW
        XCTAssertFalse(ConnectedServicesPolicy.isAvailable)
        #else
        XCTAssertTrue(ConnectedServicesPolicy.isAvailable)
        #endif
    }

    #if !APP_STORE_REVIEW
    func testSpendExtLabel() {
        XCTAssertEqual(CoinCopy.ledgerKind("spend_ext"), "外部サービス")
        LanguagePreference.shared.setting = .en
        XCTAssertEqual(CoinCopy.ledgerKind("spend_ext"), "External services")
    }

    private static let gcal = ExtProvider(id: "googleCalendar", nameJa: "Google カレンダー", nameEn: "Google Calendar")
    private static let todoist = ExtProvider(id: "todoist", nameJa: "Todoist", nameEn: "Todoist")
    private static let fake = ExtProvider(id: "fakeOauth", nameJa: "試験用のサービス", nameEn: "Test service")

    func testStateMapsConnectionsThenProviders() {
        let list = ExtConnectionList(connections: [
            ExtConnection(provider: "todoist", status: .error),
            ExtConnection(provider: "someNew", status: .active),
            ExtConnection(provider: "googleCalendar", status: .active),
        ], providers: [Self.gcal, Self.todoist, Self.fake])
        let state = ConnectedServicesState.from(.success(list), english: false)
        XCTAssertEqual(state, .loaded([
            ConnectedServiceRow(provider: "googleCalendar", name: "Google カレンダー", status: .connected),
            ConnectedServiceRow(provider: "todoist", name: "Todoist", status: .needsReconnect),
            ConnectedServiceRow(provider: "someNew", name: "someNew", status: .connected, enabled: false),
            ConnectedServiceRow(provider: "fakeOauth", name: "試験用のサービス", status: .notConnected),
        ]))
    }

    func testNamesFollowLanguage() {
        let list = ExtConnectionList(connections: [], providers: [Self.gcal, Self.fake])
        XCTAssertEqual(ConnectedServicesState.rows(list, english: true).map(\.name), ["Google Calendar", "Test service"])
        XCTAssertEqual(ConnectedServicesState.rows(list, english: false).map(\.name), ["Google カレンダー", "試験用のサービス"])
    }

    func testEmptyListShowsProviders() {
        let list = ExtConnectionList(connections: [], providers: [Self.gcal, Self.todoist])
        XCTAssertEqual(ConnectedServicesState.from(.success(list), english: false),
                       .loaded([ConnectedServiceRow(provider: "googleCalendar", name: "Google カレンダー", status: .notConnected),
                                ConnectedServiceRow(provider: "todoist", name: "Todoist", status: .notConnected)]))
    }

    /// 古いサーバー(providers が無い): つないだものだけ出し、つなぎ直せる。
    func testOldServerShowsOnlyConnected() {
        let list = ExtConnectionList(connections: [ExtConnection(provider: "todoist", status: .error)], providers: nil)
        XCTAssertEqual(ConnectedServicesState.from(.success(list), english: false),
                       .loaded([ConnectedServiceRow(provider: "todoist", name: "Todoist", status: .needsReconnect, enabled: true)]))
        XCTAssertEqual(ConnectedServicesState.from(.success(ExtConnectionList(connections: [], providers: nil)), english: false), .loaded([]))
    }

    /// enabled == false: つないでいなければ押せない・「いまは使えません」。つないでいれば切れる(行は押せる)。
    func testDisabledProviderRows() {
        var off = Self.todoist
        off.enabled = false
        var offCal = Self.gcal
        offCal.enabled = false
        let rows = ConnectedServicesState.rows(ExtConnectionList(connections: [ExtConnection(provider: "googleCalendar", status: .active)],
                                                                 providers: [offCal, off]), english: false)
        XCTAssertEqual(rows, [
            ConnectedServiceRow(provider: "googleCalendar", name: "Google カレンダー", status: .connected, enabled: false),
            ConnectedServiceRow(provider: "todoist", name: "Todoist", status: .notConnected, enabled: false),
        ])
        XCTAssertFalse(rows[0].isUnavailable, "つないでいれば切れる")
        XCTAssertEqual(ConnectedServicesCopy.status(rows[0]), "つないでいます")
        XCTAssertTrue(rows[1].isUnavailable)
        XCTAssertEqual(ConnectedServicesCopy.status(rows[1]), "いまは使えません")
        LanguagePreference.shared.setting = .en
        XCTAssertEqual(ConnectedServicesCopy.status(rows[1]), "Not available right now")
    }

    func testUnknownStatusNeedsReconnect() {
        let list = ExtConnectionList(connections: [ExtConnection(provider: "todoist", status: .unknown("x"))], providers: [Self.todoist])
        XCTAssertEqual(ConnectedServicesState.from(.success(list), english: false),
                       .loaded([ConnectedServiceRow(provider: "todoist", name: "Todoist", status: .needsReconnect)]))
    }

    func testExtUnavailableHidesSection() {
        let off = APIError.server(status: 503, code: "ext_unavailable", message: "")
        XCTAssertEqual(ConnectedServicesState.from(.failure(off)), .hidden)
        let busy = APIError.server(status: 503, code: "ext_busy", message: "")
        XCTAssertEqual(ConnectedServicesState.from(.failure(busy)), .failed("一覧を読み込めませんでした。"))
    }

    func testStatusCopy() {
        XCTAssertEqual(ConnectedServicesCopy.status(.needsReconnect), "つなぎ直してください")
        LanguagePreference.shared.setting = .en
        XCTAssertEqual(ConnectedServicesCopy.status(.connected), "Connected")
        XCTAssertEqual(ConnectedServicesCopy.status(.notConnected), "Not connected")
    }

    /// この画面の supportedFeatures: 端末は provider の一覧を持たず絞らない(nil)。審査用の版は空(= サーバーは何も出さない)。
    func testSupportedFeaturesForThisScreen() {
        XCTAssertEqual(ConnectedServicesFeatures.list(reviewBuild: true), [])
        XCTAssertNil(ConnectedServicesFeatures.list(reviewBuild: false))
        #if APP_STORE_REVIEW
        XCTAssertEqual(ConnectedServicesFeatures.current, [])
        #else
        XCTAssertNil(ConnectedServicesFeatures.current, "サーバーが登録している provider をそのまま出す")
        #endif
        // generate に送る一覧には足さない(外を呼ぶ DSL の部品はまだ無い)
        XCTAssertFalse(ClientCapabilities.supportedFeatures.contains { $0.hasPrefix("externalCall.") })
    }

    /// サーバーの名前が無い時は端末の名前(ja/en)。知らない provider は id のまま・アイコンは汎用。
    func testCatalogNamesAndSymbols() {
        XCTAssertTrue(ConnectedServiceCatalog.knownIDs.count >= 20)
        for id in ConnectedServiceCatalog.knownIDs {
            XCTAssertNotEqual(ConnectedServiceCatalog.name(id, english: false), id, id)
            XCTAssertNotEqual(ConnectedServiceCatalog.name(id, english: true), id, id)
            XCTAssertNotEqual(ConnectedServiceCatalog.symbol(id), ConnectedServiceCatalog.fallbackSymbol, id)
            XCTAssertNotNil(UIImage(systemName: ConnectedServiceCatalog.symbol(id)), id)
        }
        XCTAssertEqual(ConnectedServiceCatalog.name("googleTasks", english: false), "Google ToDo リスト")
        XCTAssertEqual(ConnectedServiceCatalog.name("googleTasks", english: true), "Google Tasks")
        XCTAssertEqual(ConnectedServiceCatalog.name("github", english: true), "GitHub")
        XCTAssertEqual(ConnectedServiceCatalog.name("someNew", english: false), "someNew")
        XCTAssertEqual(ConnectedServiceCatalog.symbol("someNew"), ConnectedServiceCatalog.fallbackSymbol)
        XCTAssertNotNil(UIImage(systemName: ConnectedServiceCatalog.fallbackSymbol))
    }

    /// サーバーの名前が空の新しい provider は端末の名前で出す。サーバーの名前があればそちら。
    func testNewProvidersRowNames() {
        let list = ExtConnectionList(connections: [ExtConnection(provider: "linear", status: .active)],
                                     providers: [ExtProvider(id: "linear", nameJa: "", nameEn: ""),
                                                 ExtProvider(id: "asana", nameJa: "アサナ", nameEn: "Asana!"),
                                                 ExtProvider(id: "someNew", nameJa: "", nameEn: "")])
        XCTAssertEqual(ConnectedServicesState.rows(list, english: false).map(\.name), ["Linear", "アサナ", "someNew"])
        XCTAssertEqual(ConnectedServicesState.rows(list, english: true).map(\.name), ["Linear", "Asana!", "someNew"])
    }

    func testLoadSendsSupportedFeatures() async {
        let mock = MockExternalServicesClient()
        let model = ConnectedServicesModel(cache: nil)
        await model.load(mock)
        let sent = await mock.lastSupportedFeatures
        XCTAssertEqual(sent, .some(ConnectedServicesFeatures.current))
        guard case .loaded(let rows) = model.state else { return XCTFail("\(model.state)") }
        XCTAssertEqual(rows.connectable.map(\.provider).sorted(), ["fakeOauth", "googleCalendar", "todoist"], "サーバーが返した provider をそのまま")
        XCTAssertEqual(rows.keyless.map(\.provider), ["frankfurter", "openWeather"])
        XCTAssertFalse(model.stale)
    }

    // MARK: - 鍵なしで使えるもの

    private static let frankfurter = ExtProvider(id: "frankfurter", nameJa: "", nameEn: "")
    private static let weather = ExtProvider(id: "openWeather", nameJa: "OpenWeather", nameEn: "OpenWeather", enabled: false)

    /// つないだもの → つなげるもの → 鍵なしで使えるもの(status .keyless)。名前はサーバー → 端末 → id。
    func testKeylessRowsComeLast() {
        let list = ExtConnectionList(connections: [ExtConnection(provider: "todoist", status: .active)],
                                     providers: [Self.gcal, Self.todoist],
                                     keyless: [Self.frankfurter, Self.weather, ExtProvider(id: "someFree", nameJa: "", nameEn: "")])
        let rows = ConnectedServicesState.rows(list, english: false)
        XCTAssertEqual(rows, [
            ConnectedServiceRow(provider: "todoist", name: "Todoist", status: .connected),
            ConnectedServiceRow(provider: "googleCalendar", name: "Google カレンダー", status: .notConnected),
            ConnectedServiceRow(provider: "frankfurter", name: "為替レート(Frankfurter)", status: .keyless),
            ConnectedServiceRow(provider: "openWeather", name: "OpenWeather", status: .keyless, enabled: false),
            ConnectedServiceRow(provider: "someFree", name: "someFree", status: .keyless),
        ])
        XCTAssertEqual(rows.connectable.map(\.provider), ["todoist", "googleCalendar"])
        XCTAssertEqual(rows.keyless.map(\.provider), ["frankfurter", "openWeather", "someFree"])
        XCTAssertEqual(ConnectedServicesState.rows(list, english: true)[2].name, "Exchange rates (Frankfurter)")
        // 古いサーバー(keyless 無し)は鍵なしの行が無い
        XCTAssertTrue(ConnectedServicesState.rows(ExtConnectionList(connections: [], providers: [Self.gcal]), english: false).keyless.isEmpty)
    }

    /// 鍵なしの行の文言: 使える / 止まっている(いまは使えません)。ボタンは無い(行は押せない扱い)
    func testKeylessCopy() {
        let on = ConnectedServiceRow(provider: "frankfurter", name: "為替", status: .keyless)
        let off = ConnectedServiceRow(provider: "openWeather", name: "天気", status: .keyless, enabled: false)
        XCTAssertFalse(on.isUnavailable)
        XCTAssertTrue(off.isUnavailable)
        XCTAssertEqual(ConnectedServicesCopy.status(on), "鍵なしで使えます")
        XCTAssertEqual(ConnectedServicesCopy.status(off), "いまは使えません")
        LanguagePreference.shared.setting = .en
        XCTAssertEqual(ConnectedServicesCopy.status(on), "No sign-in needed")
    }

    // MARK: - 控え(オフライン)

    private func tempCache() -> (ExtConnectionListCache, URL) {
        let dir = FileManager.default.temporaryDirectory.appendingPathComponent("metamon-test-\(UUID().uuidString)")
        return (ExtConnectionListCache(directory: dir), dir)
    }

    /// 読めた一覧は控え、読めない時は控えを stale で出す。読めたら stale が消える。
    func testLoadFailureFallsBackToCache() async {
        let (cache, dir) = tempCache()
        defer { try? FileManager.default.removeItem(at: dir) }
        let mock = MockExternalServicesClient(connections: [ExtConnection(provider: "todoist", status: .active)])
        let model = ConnectedServicesModel(cache: cache)
        await model.load(mock)
        XCTAssertEqual(cache.load()?.connections.map(\.provider), ["todoist"], "読めた一覧を控える")

        await mock.setNextError(URLError(.notConnectedToInternet))
        let offline = ConnectedServicesModel(cache: cache)
        await offline.load(mock)
        guard case .loaded(let rows) = offline.state else { return XCTFail("\(offline.state)") }
        XCTAssertEqual(rows.first, ConnectedServiceRow(provider: "todoist", name: "Todoist", status: .connected))
        XCTAssertTrue(offline.stale)
        XCTAssertTrue(offline.providersKnown)
        XCTAssertEqual(ConnectedServicesCopy.footer(offline), "サーバーに届かないため、前に読み込んだ一覧を出しています。")

        await offline.load(mock)
        XCTAssertFalse(offline.stale, "読めたら控えではなくなる")
        XCTAssertEqual(ConnectedServicesCopy.footer(offline), "カレンダーやタスクなどの外のサービスとつなぐと、アプリから使えます。Pro で使えます。")
        offline.message = "x"
        XCTAssertEqual(ConnectedServicesCopy.footer(offline), "x", "操作の失敗が最優先")
    }

    /// 控えが無ければ再試行できる失敗。ext_unavailable は控えがあっても隠す。
    func testLoadFailureWithoutCacheIsRetryable() async {
        let (cache, dir) = tempCache()
        defer { try? FileManager.default.removeItem(at: dir) }
        let mock = MockExternalServicesClient()
        let model = ConnectedServicesModel(cache: cache)
        await mock.setNextError(URLError(.timedOut))
        await model.load(mock)
        XCTAssertEqual(model.state, .failed("一覧を読み込めませんでした。"))
        XCTAssertFalse(model.stale)
        await model.load(mock)
        guard case .loaded = model.state else { return XCTFail("再試行で読み込める") }

        await mock.setNextError(APIError.server(status: 503, code: "ext_unavailable", message: ""))
        await model.load(mock)
        XCTAssertEqual(model.state, .hidden, "控えがあっても止めている時は隠す")
    }

    func testCallbackParse() {
        XCTAssertEqual(ConnectCallback.parse(URL(string: "tailorit://connected?provider=todoist&status=ok")!), .ok(provider: "todoist"))
        XCTAssertEqual(ConnectCallback.parse(URL(string: "tailorit://connected?provider=todoist&status=error")!), .error(provider: "todoist"))
        XCTAssertEqual(ConnectCallback.parse(URL(string: "tailorit://import/abc")!), .invalid)
        XCTAssertEqual(ConnectCallback.parse(URL(string: "https://connected?status=ok")!), .invalid)
    }

    func testConnectRefreshesListAfterReturn() async {
        let mock = MockExternalServicesClient()
        let model = ConnectedServicesModel(cache: nil)
        await model.load(mock)
        await model.connect("todoist", client: mock) { _ in
            XCTFail("モックは同意の画面を開かない")
            return URL(string: "tailorit://connected?status=ok")!
        }
        guard case .loaded(let rows) = model.state else { return XCTFail("\(model.state)") }
        XCTAssertEqual(rows.first, ConnectedServiceRow(provider: "todoist", name: "Todoist", status: .connected))
        XCTAssertNil(model.busy)
        XCTAssertNil(model.message)
    }

    func testDisconnectRemovesRow() async {
        let mock = MockExternalServicesClient(connections: [ExtConnection(provider: "todoist", status: .active)])
        let model = ConnectedServicesModel(cache: nil)
        await model.disconnect("todoist", client: mock)
        let disconnected = await mock.disconnected
        XCTAssertEqual(disconnected, ["todoist"])
        guard case .loaded(let rows) = model.state else { return XCTFail("\(model.state)") }
        XCTAssertFalse(rows.connectable.contains { $0.status != .notConnected })
    }

    func testConnectExtUnavailableHides() async {
        let mock = MockExternalServicesClient()
        let model = ConnectedServicesModel(cache: nil)
        await model.load(mock)
        await mock.setNextError(APIError.server(status: 503, code: "ext_unavailable", message: ""))
        await model.connect("todoist", client: mock) { $0 }
        XCTAssertEqual(model.state, .hidden)
    }

    func testCallbackErrorShowsMessage() async {
        let model = ConnectedServicesModel(cache: nil)
        let live = StubStartClient()
        await model.connect("todoist", client: live) { _ in URL(string: "tailorit://connected?provider=todoist&status=error")! }
        XCTAssertEqual(model.message, "つなげませんでした。もう一度試してください。")
    }

    /// 読めない戻り先・頼んだのと別の provider の ok は成功扱いにしない
    func testUnexpectedCallbackShowsMessage() async {
        let failed = "つなげませんでした。もう一度試してください。"
        for back in ["tailorit://import/abc", "tailorit://connected?provider=notion&status=ok", "tailorit://connected?status=ok"] {
            let model = ConnectedServicesModel(cache: nil)
            await model.connect("todoist", client: StubStartClient()) { _ in URL(string: back)! }
            XCTAssertEqual(model.message, failed, back)
        }
        let model = ConnectedServicesModel(cache: nil)
        await model.connect("todoist", client: StubStartClient()) { _ in URL(string: "tailorit://connected?provider=todoist&status=ok")! }
        XCTAssertNil(model.message)
    }

    func testCallbackSucceededOnlyForSameProvider() {
        XCTAssertTrue(ConnectCallback.ok(provider: "todoist").succeeded(for: "todoist"))
        XCTAssertFalse(ConnectCallback.ok(provider: "notion").succeeded(for: "todoist"))
        XCTAssertFalse(ConnectCallback.ok(provider: nil).succeeded(for: "todoist"))
        XCTAssertFalse(ConnectCallback.error(provider: "todoist").succeeded(for: "todoist"))
        XCTAssertFalse(ConnectCallback.invalid.succeeded(for: "todoist"))
    }

    func testUserCancelIsSilent() async {
        let model = ConnectedServicesModel(cache: nil)
        await model.connect("todoist", client: StubStartClient()) { _ in
            throw NSError(domain: ASWebAuthenticationSessionErrorDomain, code: ASWebAuthenticationSessionError.Code.canceledLogin.rawValue)
        }
        XCTAssertNil(model.message)
    }
    #endif
}

#if !APP_STORE_REVIEW
/// 本物の同意の URL を返すだけの口(モックの近道を通らない)。
private struct StubStartClient: ExternalServicesClient {
    func connections(supportedFeatures: [String]?) async throws -> ExtConnectionList { ExtConnectionList(connections: [], providers: []) }
    func startConnection(provider: String, scopes: [String]?) async throws -> URL { URL(string: "https://auth.example.com/\(provider)")! }
    func disconnect(provider: String) async throws {}
    func call(appId: String, provider: String, op: String, args: [String: String], requestId: String?) async throws -> ExtCallResponse {
        ExtCallResponse(result: ExtResult())
    }
}
#endif
