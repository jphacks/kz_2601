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
    private var releases = 0

    override func setUp() async throws {
        suite = "MacPushTests.\(UUID().uuidString)"
        defaults = UserDefaults(suiteName: suite)
        MacPushHost.shared.install(client: client, defaults: defaults, environment: .sandbox, appVersion: "1.0",
                                   uid: { [unowned self] in uid }, authorization: { [unowned self] in status },
                                   requestToken: { [unowned self] in requests += 1 }, openApp: { [unowned self] in opened.append($0) },
                                   releaseToken: { [unowned self] in releases += 1 })
    }

    override func tearDown() async throws {
        MacPushHost.shared.install(client: nil, defaults: .standard, environment: .production, uid: { nil },
                                   authorization: { .notDetermined }, requestToken: {}, openApp: { _ in }, releaseToken: {})
        _ = MacNotificationRouter.shared.take()
        MacNotificationRouter.shared.now = Date.init
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

    /// サインアウトの後は次のサインイン(uid の変化)までトークンを求めない・遅れて届いても送らない
    func testUnregisterStopsRegisteringUntilTheNextUser() async {
        await MacPushHost.shared.receive(token: token)
        await MacPushHost.shared.unregister()
        XCTAssertEqual(releases, 1, "OS のトークンも手放す")
        await MacPushHost.shared.refresh()
        XCTAssertEqual(requests, 0, "同じ利用者では求めない")
        await MacPushHost.shared.receive(token: token)
        var puts = await client.puts.count
        XCTAssertEqual(puts, 1, "遅れて届いたトークンで前の利用者を登録し直さない")

        uid = "u2"
        await MacPushHost.shared.refresh()
        XCTAssertEqual(requests, 1, "次の利用者では求める")
        await MacPushHost.shared.receive(token: token)
        puts = await client.puts.count
        XCTAssertEqual(puts, 2)
        uid = "u1"
        await MacPushHost.shared.receive(token: token)
        puts = await client.puts.count
        XCTAssertEqual(puts, 3, "一度 uid が変われば止めるのは終わり")
    }

    func testResumeAfterAFailedSwitchRegistersTheSameUserAgain() async {
        await MacPushHost.shared.receive(token: token)
        await MacPushHost.shared.unregister()
        MacPushHost.shared.resume()
        await MacPushHost.shared.refresh()
        XCTAssertEqual(requests, 1)
        await MacPushHost.shared.receive(token: token)
        let puts = await client.puts.count
        XCTAssertEqual(puts, 2)
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

    /// 起動直後のタップ: サーバーの一覧が載る前は行き先を残し、載ったら開く
    func testPushRouteWaitsUntilTheAppIsListed() {
        let router = MacNotificationRouter.shared
        router.open(appId: "app2")
        XCTAssertNil(router.resolve(among: ["app1"]), "まだ一覧に無い")
        XCTAssertEqual(router.pending, .app("app2"), "消さずに待つ")
        XCTAssertEqual(router.resolve(among: ["app1", "app2"]), "app2", "載ったら開く")
        XCTAssertNil(router.pending)
    }

    func testPushRouteGivesUpAfterTheListIsRefreshed() {
        let router = MacNotificationRouter.shared
        router.open(appId: "gone")
        XCTAssertNil(router.resolve(among: ["app1"], listRefreshed: true))
        XCTAssertNil(router.pending, "取り直しても無ければ諦める")
        XCTAssertNil(router.resolve(among: ["app1", "gone"]), "後から載っても開かない")
    }

    func testPushRouteGivesUpAfterALongTime() {
        let router = MacNotificationRouter.shared
        var clock = Date(timeIntervalSince1970: 1000)
        router.now = { clock }
        router.open(appId: "late")
        clock += MacNotificationRouter.appRouteLifetime + 1
        XCTAssertNil(router.resolve(among: ["app1", "late"]), "待つ長さを過ぎたら一覧に載っても開かない")
        XCTAssertNil(router.pending)
    }

    func testLocalNotificationRouteIsResolvedOnce() {
        let router = MacNotificationRouter.shared
        router.open(notificationId: "other")
        XCTAssertNil(router.resolve(among: ["app1"]))
        XCTAssertNil(router.pending, "ローカル通知は今までどおり 1 回で消す")
    }

    // MARK: - アカウント削除の失敗

    private struct FailingAccountClient: AccountClient {
        func deleteAccount() async throws { throw URLError(.notConnectedToInternet) }
    }

    /// 削除が失敗したら(サインインしたまま)端末の登録を戻す
    func testFailedAccountDeletionRegistersPushAgain() async {
        await MacPushHost.shared.receive(token: token)
        let account = MacAccount(session: AuthSession(service: MockAuthService(signedIn: true)), account: FailingAccountClient(), isLive: false)
        account.appleReauth = { nil }
        do {
            _ = try await account.deleteAccount()
            XCTFail("サーバーで消せなければ throw")
        } catch {}
        let deletes = await client.deletes
        XCTAssertEqual(deletes, 1, "削除の前に登録を消す")
        XCTAssertEqual(requests, 1, "失敗したらトークンを求め直す")
        await MacPushHost.shared.receive(token: token)
        let puts = await client.puts.count
        XCTAssertEqual(puts, 2, "同じ利用者で登録し直す")
    }

    func testPayloadAppId() {
        XCTAssertEqual(PushPayload.appId(from: ["appId": "app1", "aps": ["alert": ["title": "t"]]]), "app1")
        XCTAssertNil(PushPayload.appId(from: ["aps": [:]]), "ローカル通知には appId が無い")
    }

    // MARK: - 同意(#339)

    /// 送っている間に利用者が変わる相手
    private final class SwitchingConsentClient: PushDeviceClient, @unchecked Sendable {
        let during: @MainActor @Sendable () -> Void
        init(during: @escaping @MainActor @Sendable () -> Void) { self.during = during }
        func putDevice(installationId: String, registration: PushDeviceRegistration) async throws {}
        func deleteDevice(installationId: String) async throws {}
        func setNotifyConsent(appId: String, allowed: Bool) async throws { await during() }
    }

    func testConsentIsNotWrittenAfterTheUserChanged() async {
        let switching = SwitchingConsentClient { [unowned self] in uid = "u2" }
        MacPushHost.shared.install(client: switching, defaults: defaults, environment: .sandbox,
                                   uid: { [unowned self] in uid }, authorization: { .authorized }, requestToken: {},
                                   openApp: { _ in }, releaseToken: {})
        let ok = await MacPushHost.shared.setConsent(true, appId: "app1")
        XCTAssertFalse(ok)
        XCTAssertFalse(MacPushHost.shared.consentAllowed(appId: "app1"), "次の利用者の控えは変わらない")
        uid = "u1"
        XCTAssertFalse(MacPushHost.shared.consentAllowed(appId: "app1"), "前の利用者の控えも書き戻さない")
    }

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
