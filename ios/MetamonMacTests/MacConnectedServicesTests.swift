#if !APP_STORE_REVIEW
import AuthenticationServices
import XCTest
import MetamonRenderer
import MetamonServices
@testable import Metamon

/// #1714: Mac の設定の「つないだサービス」。一覧・つなぐ(同意の URL と戻り先)・切る・失敗の状態・戻り先の受け口
@MainActor
final class MacConnectedServicesTests: XCTestCase {
    /// 呼ばれ方を覚える偽のクライアント
    private final class FakeClient: ExternalServicesClient, @unchecked Sendable {
        var list = ExtConnectionList(connections: [], providers: [
            ExtProvider(id: "googleCalendar", nameJa: "Google カレンダー", nameEn: "Google Calendar"),
            ExtProvider(id: "notion", nameJa: "Notion", nameEn: "Notion"),
        ])
        var authURL = URL(string: "https://runner.example/oauth/start/notion?state=abc")!
        var listError: Error?
        var startError: Error?
        var disconnectError: Error?
        var listCalls = 0
        var supportedFeatures: [String]??
        var started: [String] = []
        var disconnected: [String] = []

        func connections(supportedFeatures: [String]?) async throws -> ExtConnectionList {
            listCalls += 1
            self.supportedFeatures = .some(supportedFeatures)
            if let listError { throw listError }
            return list
        }

        func startConnection(provider: String, scopes: [String]?) async throws -> URL {
            started.append(provider)
            if let startError { throw startError }
            return authURL
        }

        func disconnect(provider: String) async throws {
            disconnected.append(provider)
            if let disconnectError { throw disconnectError }
            list.connections.removeAll { $0.provider == provider }
        }

        func call(appId: String, provider: String, op: String, args: [String: String], requestId: String?) async throws -> ExtCallResponse {
            ExtCallResponse(result: ExtResult())
        }
    }

    private var saved: AppLanguage = .system

    override func setUp() {
        saved = LanguagePreference.shared.setting
        LanguagePreference.shared.setting = .ja
    }

    override func tearDown() { LanguagePreference.shared.setting = saved }

    private static let unavailable = APIError.server(status: 503, code: APIError.extUnavailableCode, message: "", details: [:])

    // MARK: - 一覧

    func testListShowsConnectedThenProviders() async {
        let client = FakeClient()
        client.list.connections = [ExtConnection(provider: "notion", status: .active)]
        let model = ConnectedServicesModel()
        await model.load(client)
        XCTAssertEqual(model.state, .loaded([
            ConnectedServiceRow(provider: "notion", name: "Notion", status: .connected),
            ConnectedServiceRow(provider: "googleCalendar", name: "Google カレンダー", status: .notConnected),
        ]))
        // この画面で扱える provider だけに絞って聞く(iOS と同じ)
        XCTAssertEqual(client.supportedFeatures, .some(ConnectedServicesFeatures.current))
    }

    func testListFailureIsRetryable() async {
        let client = FakeClient()
        client.listError = URLError(.notConnectedToInternet)
        let model = ConnectedServicesModel()
        await model.load(client)
        XCTAssertEqual(model.state, .failed("一覧を読み込めませんでした。"))
        client.listError = nil
        await model.load(client)
        guard case .loaded = model.state else { return XCTFail("再試行で読み込める") }
    }

    func testExtUnavailableHides() async {
        let client = FakeClient()
        client.listError = Self.unavailable
        let model = ConnectedServicesModel()
        await model.load(client)
        XCTAssertEqual(model.state, .hidden)
    }

    // MARK: - つなぐ

    func testConnectOpensAuthURLAndReloads() async {
        let client = FakeClient()
        let model = ConnectedServicesModel()
        var opened: [URL] = []
        await model.connect("notion", client: client) { url in
            opened.append(url)
            return URL(string: "tailorit://connected?provider=notion&status=ok")!
        }
        XCTAssertEqual(client.started, ["notion"])
        XCTAssertEqual(opened, [client.authURL], "サーバーの同意の URL をそのまま開く")
        XCTAssertNil(model.message)
        XCTAssertEqual(client.listCalls, 1, "戻ったら一覧を取り直す")
        XCTAssertNil(model.busy)
    }

    func testConnectErrorCallbackShowsMessage() async {
        let client = FakeClient()
        let model = ConnectedServicesModel()
        await model.connect("notion", client: client) { _ in URL(string: "tailorit://connected?provider=notion&status=error")! }
        XCTAssertEqual(model.message, "つなげませんでした。もう一度試してください。")
    }

    func testConnectCancelIsSilent() async {
        let client = FakeClient()
        let model = ConnectedServicesModel()
        await model.connect("notion", client: client) { _ in throw ASWebAuthenticationSessionError(.canceledLogin) }
        XCTAssertNil(model.message, "自分で閉じた時は何も言わない")
    }

    func testConnectStartFailure() async {
        let client = FakeClient()
        client.startError = URLError(.timedOut)
        let model = ConnectedServicesModel()
        await model.connect("notion", client: client) { _ in XCTFail("同意の画面は開かない"); return URL(string: "x:")! }
        XCTAssertEqual(model.message, "つなげませんでした。もう一度試してください。")
    }

    /// Mac の同意の画面: 戻り先のスキーム・使い回さないブラウザ・前面のウインドウに出す
    func testAuthenticatorSession() {
        let auth = MacWebAuthenticator()
        let window = NSWindow()
        auth.anchor = { window }
        let session = auth.makeSession(url: URL(string: "https://runner.example/oauth")!, callbackScheme: ConnectCallback.scheme) { _ in }
        XCTAssertTrue(session.prefersEphemeralWebBrowserSession)
        XCTAssertTrue(session.presentationContextProvider === auth)
        XCTAssertTrue(auth.presentationAnchor(for: session) === window)
    }

    // MARK: - 切る

    func testDisconnectRemovesAndReloads() async {
        let client = FakeClient()
        client.list.connections = [ExtConnection(provider: "notion", status: .error)]
        let model = ConnectedServicesModel()
        await model.disconnect("notion", client: client)
        XCTAssertEqual(client.disconnected, ["notion"])
        XCTAssertNil(model.message)
        XCTAssertEqual(model.state, .loaded([
            ConnectedServiceRow(provider: "googleCalendar", name: "Google カレンダー", status: .notConnected),
            ConnectedServiceRow(provider: "notion", name: "Notion", status: .notConnected),
        ]))
    }

    func testDisconnectFailureShowsMessage() async {
        let client = FakeClient()
        client.disconnectError = URLError(.notConnectedToInternet)
        let model = ConnectedServicesModel()
        await model.disconnect("notion", client: client)
        XCTAssertEqual(model.message, "切れませんでした。もう一度試してください。")
        client.disconnectError = Self.unavailable
        await model.disconnect("notion", client: client)
        XCTAssertEqual(model.state, .hidden)
    }

    func testStatusCopy() {
        XCTAssertEqual(MacConnectedServicesCopy.status(ConnectedServiceRow(provider: "notion", name: "Notion", status: .connected)), "つないでいます")
        XCTAssertEqual(MacConnectedServicesCopy.status(ConnectedServiceRow(provider: "notion", name: "Notion", status: .needsReconnect)), "つなぎ直してください")
        XCTAssertEqual(MacConnectedServicesCopy.status(ConnectedServiceRow(provider: "notion", name: "Notion", status: .notConnected, enabled: false)), "いまは使えません")
        LanguagePreference.shared.setting = .en
        XCTAssertEqual(MacConnectedServicesCopy.status(.notConnected), "Not connected")
    }

    // MARK: - 戻り先の受け口(MacLinkReceiver)

    func testInboxTakesOnlyConnectCallbacks() {
        let inbox = MacConnectCallbackInbox()
        XCTAssertFalse(inbox.receive(URL(string: "tailorit://import/abc")!), "共有のリンクは共有の流れへ")
        XCTAssertFalse(inbox.receive(URL(string: "https://tailor-it.app/s/abc")!))
        XCTAssertEqual(inbox.revision, 0)
        XCTAssertTrue(inbox.receive(URL(string: "tailorit://connected?provider=notion&status=ok")!))
        XCTAssertEqual(inbox.last, .ok(provider: "notion"))
        XCTAssertTrue(inbox.receive(URL(string: "tailorit://connected?provider=notion&status=error")!))
        XCTAssertEqual(inbox.revision, 2)
        XCTAssertTrue(MacConnectCallbackInbox.isFailure(inbox.last!))
        XCTAssertFalse(MacConnectCallbackInbox.isFailure(.ok(provider: nil)))
    }

    /// appconfig の URL が無い(モック)時はつながない
    func testServicesExternalOnlyWithAppConfig() {
        let mock = MacServices(auth: MockAuthService(signedIn: true), ai: MockAIClient(), feedback: MockFeedbackClient(), isLive: false)
        XCTAssertNil(mock.external)
        let live = MacServices(auth: MockAuthService(signedIn: true), ai: MockAIClient(), feedback: MockFeedbackClient(), isLive: true,
                               appConfigURL: URL(string: "https://appconfig.example/")!, token: NoAuthTokenProvider())
        XCTAssertTrue(live.external is HTTPExternalServicesClient)
    }
}
#endif
