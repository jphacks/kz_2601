import AuthenticationServices
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
                       .loaded([ConnectedServiceRow(provider: "todoist", name: "todoist", status: .needsReconnect, enabled: true)]))
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

    /// この画面だけの supportedFeatures。審査用の版は空(= サーバーは何も出さない)。
    func testSupportedFeaturesForThisScreen() {
        XCTAssertEqual(ConnectedServicesFeatures.list(reviewBuild: true, debug: true), [])
        XCTAssertEqual(ConnectedServicesFeatures.list(reviewBuild: false, debug: false),
                       ["externalCall.googleCalendar", "externalCall.todoist", "externalCall.outlookCalendar", "externalCall.notion"])
        XCTAssertEqual(ConnectedServicesFeatures.list(reviewBuild: false, debug: true).last, "externalCall.fakeOauth")
        #if DEBUG
        XCTAssertTrue(ConnectedServicesFeatures.current.contains("externalCall.fakeOauth"))
        #endif
        // generate に送る一覧には足さない(外を呼ぶ DSL の部品はまだ無い)
        XCTAssertFalse(ClientCapabilities.supportedFeatures.contains { $0.hasPrefix("externalCall.") })
    }

    func testLoadSendsSupportedFeatures() async {
        let mock = MockExternalServicesClient()
        let model = ConnectedServicesModel()
        await model.load(mock)
        let sent = await mock.lastSupportedFeatures
        XCTAssertEqual(sent, .some(ConnectedServicesFeatures.current))
        guard case .loaded(let rows) = model.state else { return XCTFail("\(model.state)") }
        XCTAssertEqual(rows.map(\.provider).sorted(), ["fakeOauth", "googleCalendar", "todoist"], "DEBUG は偽の provider も出す")
    }

    func testCallbackParse() {
        XCTAssertEqual(ConnectCallback.parse(URL(string: "tailorit://connected?provider=todoist&status=ok")!), .ok(provider: "todoist"))
        XCTAssertEqual(ConnectCallback.parse(URL(string: "tailorit://connected?provider=todoist&status=error")!), .error(provider: "todoist"))
        XCTAssertEqual(ConnectCallback.parse(URL(string: "tailorit://import/abc")!), .invalid)
        XCTAssertEqual(ConnectCallback.parse(URL(string: "https://connected?status=ok")!), .invalid)
    }

    func testConnectRefreshesListAfterReturn() async {
        let mock = MockExternalServicesClient()
        let model = ConnectedServicesModel()
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
        let model = ConnectedServicesModel()
        await model.disconnect("todoist", client: mock)
        let disconnected = await mock.disconnected
        XCTAssertEqual(disconnected, ["todoist"])
        guard case .loaded(let rows) = model.state else { return XCTFail("\(model.state)") }
        XCTAssertFalse(rows.contains { $0.status != .notConnected })
    }

    func testConnectExtUnavailableHides() async {
        let mock = MockExternalServicesClient()
        let model = ConnectedServicesModel()
        await model.load(mock)
        await mock.setNextError(APIError.server(status: 503, code: "ext_unavailable", message: ""))
        await model.connect("todoist", client: mock) { $0 }
        XCTAssertEqual(model.state, .hidden)
    }

    func testCallbackErrorShowsMessage() async {
        let model = ConnectedServicesModel()
        let live = StubStartClient()
        await model.connect("todoist", client: live) { _ in URL(string: "tailorit://connected?provider=todoist&status=error")! }
        XCTAssertEqual(model.message, "つなげませんでした。もう一度試してください。")
    }

    func testUserCancelIsSilent() async {
        let model = ConnectedServicesModel()
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
