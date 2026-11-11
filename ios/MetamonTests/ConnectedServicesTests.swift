import AuthenticationServices
import XCTest
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

    func testStateMapsConnectionsThenCandidates() {
        let list = [
            ExtConnection(provider: "todoist", status: .error),
            ExtConnection(provider: "someNew", status: .active),
            ExtConnection(provider: "googleCalendar", status: .active),
        ]
        let state = ConnectedServicesState.from(.success(list), candidates: ["googleCalendar", "todoist", "fakeOauth"])
        XCTAssertEqual(state, .loaded([
            ConnectedServiceRow(provider: "googleCalendar", status: .connected),
            ConnectedServiceRow(provider: "todoist", status: .needsReconnect),
            ConnectedServiceRow(provider: "someNew", status: .connected),
            ConnectedServiceRow(provider: "fakeOauth", status: .notConnected),
        ]))
    }

    func testEmptyListShowsCandidates() {
        XCTAssertEqual(ConnectedServicesState.from(.success([]), candidates: ["googleCalendar", "todoist"]),
                       .loaded([ConnectedServiceRow(provider: "googleCalendar", status: .notConnected),
                                ConnectedServiceRow(provider: "todoist", status: .notConnected)]))
    }

    func testUnknownStatusNeedsReconnect() {
        let state = ConnectedServicesState.from(.success([ExtConnection(provider: "todoist", status: .unknown("x"))]), candidates: [])
        XCTAssertEqual(state, .loaded([ConnectedServiceRow(provider: "todoist", status: .needsReconnect)]))
    }

    func testExtUnavailableHidesSection() {
        let off = APIError.server(status: 503, code: "ext_unavailable", message: "")
        XCTAssertEqual(ConnectedServicesState.from(.failure(off)), .hidden)
        let busy = APIError.server(status: 503, code: "ext_busy", message: "")
        XCTAssertEqual(ConnectedServicesState.from(.failure(busy)), .failed("一覧を読み込めませんでした。"))
    }

    func testProviderNamesAndStatusCopy() {
        XCTAssertEqual(ConnectedServicesCatalog.name("googleCalendar"), "Google カレンダー")
        XCTAssertEqual(ConnectedServicesCatalog.name("unknownOne"), "unknownOne")
        XCTAssertEqual(ConnectedServicesCopy.status(.needsReconnect), "つなぎ直してください")
        LanguagePreference.shared.setting = .en
        XCTAssertEqual(ConnectedServicesCatalog.name("googleCalendar"), "Google Calendar")
        XCTAssertEqual(ConnectedServicesCopy.status(.connected), "Connected")
        XCTAssertEqual(ConnectedServicesCopy.status(.notConnected), "Not connected")
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
        XCTAssertEqual(rows.first, ConnectedServiceRow(provider: "todoist", status: .connected))
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
    func connections() async throws -> [ExtConnection] { [] }
    func startConnection(provider: String, scopes: [String]?) async throws -> URL { URL(string: "https://auth.example.com/\(provider)")! }
    func disconnect(provider: String) async throws {}
    func call(appId: String, provider: String, op: String, args: [String: String], requestId: String?) async throws -> ExtCallResponse {
        ExtCallResponse(result: ExtResult())
    }
}
#endif
