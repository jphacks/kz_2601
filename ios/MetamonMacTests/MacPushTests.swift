#if !APP_STORE_REVIEW
import XCTest
import UserNotifications
import MetamonServices
@testable import Metamon

/// Mac のプッシュ(#982): 許可済みの時だけトークンを求め、共有の `PushDeviceRegistrar` でサーバーに送る・タップでそのアプリを開く。
@MainActor
final class MacPushTests: XCTestCase {
    private actor FakePushClient: PushDeviceClient {
        var puts: [(id: String, registration: PushDeviceRegistration)] = []
        var deletes = 0
        var consents: [String] = []
        var failing = false
        func set(failing: Bool) { self.failing = failing }

        func putDevice(installationId: String, registration: PushDeviceRegistration) async throws {
            if failing { throw URLError(.notConnectedToInternet) }
            puts.append((installationId, registration))
        }

        func deleteDevice(installationId: String) async throws {
            if failing { throw URLError(.notConnectedToInternet) }
            deletes += 1
        }

        func setNotifyConsent(appId: String, allowed: Bool) async throws {
            if failing { throw URLError(.notConnectedToInternet) }
            consents.append("\(appId) \(allowed)")
        }
    }

    private let client = FakePushClient()
    private var defaults: UserDefaults!
    private var suite = ""
    private var uid: String? = "u1"
    private var status = UNAuthorizationStatus.authorized
    private var requests = 0
    private var opened: [String] = []

    override func setUp() async throws {
        suite = "MacPushTests.\(UUID().uuidString)"
        defaults = UserDefaults(suiteName: suite)
        MacPushHost.shared.install(client: client, defaults: defaults, environment: .sandbox, appVersion: "1.0",
                                   uid: { [unowned self] in uid }, authorization: { [unowned self] in status },
                                   requestToken: { [unowned self] in requests += 1 }, openApp: { [unowned self] in opened.append($0) })
    }

    override func tearDown() async throws {
        MacPushHost.shared.install(client: nil, defaults: .standard, environment: .production, uid: { nil },
                                   authorization: { .notDetermined }, requestToken: {}, openApp: { _ in })
        _ = MacNotificationRouter.shared.take()
        defaults.removePersistentDomain(forName: suite)
    }

    private var token: Data { Data(repeating: 0xAB, count: 32) }

    // MARK: - トークンの登録

    func testRequestsATokenOnlyWhenAllowedAndSignedIn() async {
        await MacPushHost.shared.refresh()
        XCTAssertEqual(requests, 1)
        status = .notDetermined
        await MacPushHost.shared.refresh()
        XCTAssertEqual(requests, 1, "未決定では求めない(許可のダイアログはここでは出さない)")
        status = .authorized
        uid = nil
        await MacPushHost.shared.refresh()
        XCTAssertEqual(requests, 1, "サインアウト中は求めない")
    }

    func testUploadsTheTokenAsMacOS() async throws {
        await MacPushHost.shared.receive(token: token)
        let puts = await client.puts
        XCTAssertEqual(puts.count, 1)
        let r = try XCTUnwrap(puts.first?.registration)
        XCTAssertEqual(r.token, String(repeating: "ab", count: 32), "backend の形(小文字の 16 進)")
        XCTAssertEqual(r.environment, "sandbox")
        XCTAssertEqual(r.appVersion, "1.0")
        XCTAssertTrue(r.os.hasPrefix("macOS "), r.os)
        XCTAssertLessThanOrEqual(r.os.count, 32, "backend の os の上限")
        XCTAssertNotNil(defaults.string(forKey: PushDeviceRegistrar.installationIdKey))
    }

    func testSameTokenIsSentOnceAndAFailureIsRetried() async {
        await client.set(failing: true)
        await MacPushHost.shared.receive(token: token)
        await client.set(failing: false)
        await MacPushHost.shared.receive(token: token)
        await MacPushHost.shared.receive(token: token)
        let count = await client.puts.count
        XCTAssertEqual(count, 1, "失敗したら次で送り直し、送れた後は同じトークンを送らない")
    }

    func testNotSentWhileSignedOut() async {
        uid = nil
        await MacPushHost.shared.receive(token: token)
        let count = await client.puts.count
        XCTAssertEqual(count, 0)
    }

    func testDeniedAfterRegisteringDeletesTheDevice() async {
        await MacPushHost.shared.receive(token: token)
        status = .denied
        await MacPushHost.shared.refresh()
        let deletes = await client.deletes
        XCTAssertEqual(deletes, 1)
    }

    func testUnregisterDeletesTheDevice() async {
        await MacPushHost.shared.receive(token: token)
        await MacPushHost.shared.unregister()
        let deletes = await client.deletes
        XCTAssertEqual(deletes, 1)
    }

    func testMockServicesDoNotRegister() {
        XCTAssertNil(MacServices.make(environment: ["METAMON_MOCK": "1"]).pushDevices)
    }

    func testOSName() {
        XCTAssertEqual(MacPushHost.osName(OperatingSystemVersion(majorVersion: 15, minorVersion: 1, patchVersion: 0)), "macOS 15.1")
        XCTAssertEqual(MacPushHost.osName(OperatingSystemVersion(majorVersion: 14, minorVersion: 6, patchVersion: 1)), "macOS 14.6.1")
    }

    /// Mac の profile は `com.apple.developer.aps-environment`(iOS は `aps-environment`)
    func testMacProvisioningProfileEnvironment() {
        let profile = { (env: String) in Data("<key>com.apple.developer.aps-environment</key>\n\t\t<string>\(env)</string>".utf8) }
        XCTAssertEqual(PushEnvironment.from(provisioningProfile: profile("development")), .sandbox)
        XCTAssertEqual(PushEnvironment.from(provisioningProfile: profile("production")), .production)
        XCTAssertEqual(PushEnvironment.from(provisioningProfile: nil), .production, "未署名・Developer ID")
    }

    // MARK: - タップ

    func testTapOpensTheAppWhenSignedIn() async {
        await MacPushHost.shared.open(appId: "app1")
        XCTAssertEqual(opened, ["app1"])
        uid = nil
        await MacPushHost.shared.open(appId: "app2")
        XCTAssertEqual(opened, ["app1"], "サインアウト中は開かない")
    }

    func testTapGoesThroughTheNotificationRouter() async {
        MacPushHost.shared.install(client: client, defaults: defaults, environment: .sandbox, uid: { "u1" })
        await MacPushHost.shared.open(appId: "app1")
        let route = MacNotificationRouter.shared.take()
        XCTAssertEqual(route, .app("app1"))
        XCTAssertNil(MacNotificationRouter.shared.take(), "1 回だけ受け取る")
    }

    func testRouteSelectsAnAppInTheList() {
        let ids = ["app1", "app1x", "app2"]
        XCTAssertEqual(MacNotificationRouter.Route.app("app1").appId(among: ids), "app1")
        XCTAssertNil(MacNotificationRouter.Route.app("gone").appId(among: ids), "一覧に無ければ前に出すだけ")
        XCTAssertEqual(MacNotificationRouter.Route.notification("dsl.app1x.n1").appId(among: ids), "app1x", "ローカル通知は今までどおり")
        XCTAssertNil(MacNotificationRouter.Route.notification("other").appId(among: ids))
    }

    func testPayloadAppId() {
        XCTAssertEqual(PushPayload.appId(from: ["appId": "app1", "aps": ["alert": ["title": "t"]]]), "app1")
        XCTAssertNil(PushPayload.appId(from: ["aps": [:]]), "ローカル通知には appId が無い")
    }

    // MARK: - 同意(#339)

    func testConsentIsSavedOnlyWhenSent() async {
        XCTAssertFalse(MacPushHost.shared.consentAllowed(appId: "app1"), "保存が無い = 受け取らない")
        let ok = await MacPushHost.shared.setConsent(true, appId: "app1")
        XCTAssertTrue(ok)
        XCTAssertTrue(MacPushHost.shared.consentAllowed(appId: "app1"))
        await client.set(failing: true)
        let failed = await MacPushHost.shared.setConsent(false, appId: "app1")
        XCTAssertFalse(failed)
        XCTAssertTrue(MacPushHost.shared.consentAllowed(appId: "app1"), "失敗したら控えを変えない")
        let consents = await client.consents
        XCTAssertEqual(consents, ["app1 true"])
    }
}
#endif
