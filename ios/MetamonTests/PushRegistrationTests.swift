import XCTest
import UserNotifications
import MetamonDSL
import MetamonServices
@testable import Metamon

/// APNs の登録の窓口(#269): 許可済みの時だけ・サインアウトと退会で消す・タップで開く。
@MainActor
final class PushRegistrationTests: XCTestCase {
    private actor FakePushClient: PushDeviceClient {
        var calls: [String] = []
        var failing = false
        func set(failing: Bool) { self.failing = failing }

        func putDevice(installationId: String, registration: PushDeviceRegistration) async throws {
            calls.append("put")
            if failing { throw URLError(.notConnectedToInternet) }
        }

        func deleteDevice(installationId: String) async throws {
            calls.append("delete")
            if failing { throw URLError(.notConnectedToInternet) }
        }

        func setNotifyConsent(appId: String, allowed: Bool) async throws {
            calls.append("consent \(appId) \(allowed)")
            if failing { throw URLError(.notConnectedToInternet) }
        }
    }

    private final class OrderedAccount: AccountClient, @unchecked Sendable {
        let client: FakePushClient
        var deletesBeforeAccount = 0
        init(client: FakePushClient) { self.client = client }
        func deleteAccount() async throws {
            deletesBeforeAccount = await client.calls.filter { $0 == "delete" }.count
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
        suite = "PushRegistrationTests.\(UUID().uuidString)"
        defaults = UserDefaults(suiteName: suite)
        install()
    }

    override func tearDown() async throws {
        PushHost.shared.install(registrar: nil, uid: { nil }, authorization: { .notDetermined }, requestToken: {}, openApp: { _ in })
        defaults.removePersistentDomain(forName: suite)
    }

    private func install(uid fixed: String? = nil) {
        let registrar = PushDeviceRegistrar(client: client, defaults: defaults, environment: .sandbox, appVersion: "1.0", os: "iOS 26.0")
        PushHost.shared.install(registrar: registrar, uid: { [unowned self] in fixed ?? uid }, authorization: { [unowned self] in status },
                                requestToken: { [unowned self] in requests += 1 }, openApp: { [unowned self] in opened.append($0) })
    }

    private var token: Data { Data(repeating: 0xAB, count: 32) }

    func testRequestsATokenOnlyWhenAllowedAndSignedIn() async {
        await PushHost.shared.refresh()
        XCTAssertEqual(requests, 1)
        for denied in [UNAuthorizationStatus.denied, .notDetermined] {
            status = denied
            await PushHost.shared.refresh()
        }
        XCTAssertEqual(requests, 1, "拒否・未決定では求めない")
        status = .authorized
        uid = nil
        await PushHost.shared.refresh()
        XCTAssertEqual(requests, 1, "サインアウト中は求めない")
    }

    func testTokenIsSentForTheSignedInUserOnly() async {
        uid = nil
        await PushHost.shared.receive(token: token)
        var calls = await client.calls
        XCTAssertEqual(calls, [])
        uid = "u1"
        await PushHost.shared.receive(token: token)
        await PushHost.shared.receive(token: token)
        calls = await client.calls
        XCTAssertEqual(calls, ["put"])
    }

    func testRevokedPermissionDeletesTheDevice() async {
        await PushHost.shared.receive(token: token)
        status = .denied
        await PushHost.shared.refresh()
        let calls = await client.calls
        XCTAssertEqual(calls, ["put", "delete"])
    }

    func testTapOpensTheAppOnlyWhenSignedIn() async {
        await PushHost.shared.open(appId: "app_1")
        uid = nil
        await PushHost.shared.open(appId: "app_2")
        XCTAssertEqual(opened, ["app_1"])
    }

    func testPresenterHandlesTheTap() {
        let selector = #selector(UNUserNotificationCenterDelegate.userNotificationCenter(_:didReceive:withCompletionHandler:))
        XCTAssertTrue(ForegroundNotificationPresenter.shared.responds(to: selector))
    }

    func testInstallationIdIsKeptByTheWipe() {
        XCTAssertTrue(LocalWipe.preservedKeys.contains(PushDeviceRegistrar.installationIdKey))
    }

    // MARK: - サインアウト・退会

    private func runtime() throws -> (AppRuntime, AuthSession, Services) {
        var services = BuildConfig.make(environment: .mock)
        services.pushDevices = client
        let rt = AppRuntime(document: try DSLExamples.document("kakeibo", version: 1), services: services)
        return (rt, AuthSession(service: services.auth), services)
    }

    func testSignOutDeletesTheDeviceAndAFailureDoesNotStopIt() async throws {
        let (rt, session, _) = try runtime()
        install(uid: "u1")
        await PushHost.shared.receive(token: token)
        await client.set(failing: true)
        let outcome = try await session.signOutAndWipe(runtime: rt, choice: .discardAndWipe)
        XCTAssertEqual(outcome, .signedOut)
        let calls = await client.calls
        XCTAssertEqual(calls, ["put", "delete"])
    }

    func testAccountDeletionDeletesTheDeviceFirst() async throws {
        let (rt, session, _) = try runtime()
        install(uid: "u1")
        await PushHost.shared.receive(token: token)
        let account = OrderedAccount(client: client)
        let deleted = try await session.deleteAccount(account: account, runtime: rt)
        XCTAssertTrue(deleted)
        XCTAssertEqual(account.deletesBeforeAccount, 1, "DELETE /v1/me の後は送れない")
    }

    /// 匿名 → Apple の引き継ぎが別のアカウントへの切り替えになる時も、切り替えの前に端末の登録を消す(Android #1301 と同じ)
    func testSwitchingToAnotherAccountDeletesTheDeviceFirst() async throws {
        let (rt, _, _) = try runtime()
        install(uid: "u1")
        await PushHost.shared.receive(token: token)
        let auth = MockAuthService(signedIn: true)
        auth.appleIDInUse = true
        let session = AuthSession(service: auth)
        _ = await session.linkApple(idToken: "t", nonce: "n", fullName: nil)
        XCTAssertTrue(session.linkConflict)

        var uidAtUnregister: String?
        let switched = await session.switchToExistingAppleAccount(beforeSwitch: {
            uidAtUnregister = session.user?.uid
            await rt.unregisterPushDevice()
        })
        XCTAssertTrue(switched)
        XCTAssertEqual(uidAtUnregister, MockAuthService.fixedUID, "匿名の uid のうちに消す")
        XCTAssertEqual(session.user?.uid, MockAuthService.existingAppleUID)
        let calls = await client.calls
        XCTAssertEqual(calls, ["put", "delete"])
    }

    // MARK: - 同意

    func testConsentIsRememberedOnlyWhenSaved() async throws {
        let (rt, _, services) = try runtime()
        let uid = try XCTUnwrap(services.auth.currentUser?.uid)
        let appId = rt.document.appId
        defer { NotifyConsentStore().set(false, uid: uid, appId: appId) }
        XCTAssertFalse(rt.notifyConsentAllowed)
        let saved = await rt.setNotifyConsent(true, appId: appId)
        XCTAssertTrue(saved)
        XCTAssertTrue(rt.notifyConsentAllowed)

        await client.set(failing: true)
        let failed = await rt.setNotifyConsent(false, appId: appId)
        XCTAssertFalse(failed)
        XCTAssertTrue(rt.notifyConsentAllowed, "送れなければ控えを変えない")
        let calls = await client.calls
        XCTAssertEqual(calls, ["consent \(appId) true", "consent \(appId) false"])
    }

    func testConsentToggleOnlyForSharedApps() async throws {
        let (rt, _, _) = try runtime() // kakeibo は private
        let shownForPrivate = await rt.canSetNotifyConsent()
        XCTAssertFalse(shownForPrivate)

        var services = BuildConfig.make(environment: .mock)
        services.pushDevices = client
        let shared = AppRuntime(document: try DSLExamples.document("board", version: 1), services: services)
        let shownForShared = await shared.canSetNotifyConsent()
        XCTAssertTrue(shownForShared)
    }
}
