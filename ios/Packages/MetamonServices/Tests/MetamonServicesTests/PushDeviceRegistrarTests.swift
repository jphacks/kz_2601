import XCTest
@testable import MetamonServices

/// 端末トークンの登録(#269)。HTTP は `StubURLProtocol`、連打や失敗は `FakePushClient`。
final class PushDeviceRegistrarTests: XCTestCase {
    private let token = String(repeating: "ab", count: 32)
    private var defaults: UserDefaults!
    private var suite = ""

    override func setUp() {
        suite = "PushDeviceRegistrarTests.\(UUID().uuidString)"
        defaults = UserDefaults(suiteName: suite)
    }

    override func tearDown() { defaults.removePersistentDomain(forName: suite) }

    private actor FakePushClient: PushDeviceClient {
        var puts: [(id: String, registration: PushDeviceRegistration)] = []
        var deletes: [String] = []
        var failing = false
        var hanging = false

        func set(failing: Bool = false, hanging: Bool = false) {
            self.failing = failing
            self.hanging = hanging
        }

        func putDevice(installationId: String, registration: PushDeviceRegistration) async throws {
            if failing { throw URLError(.notConnectedToInternet) }
            puts.append((installationId, registration))
        }

        func deleteDevice(installationId: String) async throws {
            deletes.append(installationId)
            if hanging { try await Task.sleep(for: .seconds(60)) }
            if failing { throw URLError(.notConnectedToInternet) }
        }

        func setNotifyConsent(appId: String, allowed: Bool) async throws {}
    }

    private func registrar(_ client: any PushDeviceClient, version: String = "1.0") -> PushDeviceRegistrar {
        PushDeviceRegistrar(client: client, defaults: defaults, environment: .sandbox, appVersion: version, os: "iOS 26.0")
    }

    func testSendsOnlyWhenSomethingChanged() async {
        let client = FakePushClient()
        let r = registrar(client)
        await r.register(token: token, uid: "u1")
        await r.register(token: token, uid: "u1")
        var count = await client.puts.count
        XCTAssertEqual(count, 1)

        await r.register(token: String(repeating: "cd", count: 32), uid: "u1") // トークンが変わった
        await r.register(token: String(repeating: "cd", count: 32), uid: "u2") // アカウントが変わった
        await registrar(client, version: "1.1").register(token: String(repeating: "cd", count: 32), uid: "u2") // 版が変わった
        count = await client.puts.count
        XCTAssertEqual(count, 4)
        let ids = await Set(client.puts.map(\.id))
        XCTAssertEqual(ids.count, 1, "installationId は安定")
    }

    func testFailedSendIsRetriedAndDoesNotCountAsSent() async {
        let client = FakePushClient()
        await client.set(failing: true)
        let r = registrar(client)
        await r.register(token: token, uid: "u1")
        await client.set()
        await r.register(token: token, uid: "u1")
        let count = await client.puts.count
        XCTAssertEqual(count, 1)
    }

    func testInstallationIdSurvivesUnregisterAndMatchesTheBackendPattern() async throws {
        let client = FakePushClient()
        let r = registrar(client)
        await r.register(token: token, uid: "u1")
        await r.unregister()
        await r.register(token: token, uid: "u2")
        let puts = await client.puts
        let deletes = await client.deletes
        XCTAssertEqual(puts.count, 2)
        XCTAssertEqual(puts[0].id, puts[1].id)
        XCTAssertEqual(deletes, [puts[0].id])
        XCTAssertNotNil(puts[0].id.range(of: #"^[A-Za-z0-9_-]{8,64}$"#, options: .regularExpression))
    }

    func testUnregisterWithoutEverRegisteringSendsNothing() async {
        let client = FakePushClient()
        await registrar(client).unregister()
        let deletes = await client.deletes
        XCTAssertTrue(deletes.isEmpty)
    }

    func testUnregisterDoesNotWaitForAStuckRequest() async {
        let client = FakePushClient()
        let r = registrar(client)
        await r.register(token: token, uid: "u1")
        await client.set(hanging: true)
        let started = Date()
        await r.unregister(timeout: .milliseconds(100))
        XCTAssertLessThan(Date().timeIntervalSince(started), 5)
    }

    func testRegisterAfterFailedUnregisterSendsAgain() async {
        let client = FakePushClient()
        let r = registrar(client)
        await r.register(token: token, uid: "u1")
        await client.set(failing: true)
        await r.unregister() // 退会がサーバーで失敗して取りやめた時など
        await client.set()
        await r.register(token: token, uid: "u1")
        let count = await client.puts.count
        XCTAssertEqual(count, 2)
    }

    func testRevokedDeletesOnlyWhenRegistered() async {
        let client = FakePushClient()
        let r = registrar(client)
        await r.authorizationRevoked()
        var deletes = await client.deletes
        XCTAssertTrue(deletes.isEmpty)
        await r.register(token: token, uid: "u1")
        await r.authorizationRevoked()
        await r.authorizationRevoked()
        deletes = await client.deletes
        XCTAssertEqual(deletes.count, 1)
    }

    // MARK: - HTTP の形(docs/backend/openapi-appconfig.yaml)

    private func httpClient() -> HTTPPushDeviceClient {
        HTTPPushDeviceClient(baseURL: URL(string: "https://example.test/")!, token: RefreshingToken())
    }

    private func withStub(_ responses: [StubURLProtocol.Canned], _ body: () async throws -> Void) async rethrows -> URLRequest? {
        let saved = HTTP.session
        HTTP.session = HTTP.makeSession(protocolClasses: [StubURLProtocol.self])
        defer { HTTP.session = saved }
        StubURLProtocol.reset(responses)
        try await body()
        return StubURLProtocol.requests.last
    }

    private func bodyJSON(_ request: URLRequest?) throws -> [String: Any] {
        var data = request?.httpBody
        if data == nil, let stream = request?.httpBodyStream {
            stream.open()
            defer { stream.close() }
            var out = Data()
            var buffer = [UInt8](repeating: 0, count: 1024)
            while stream.hasBytesAvailable {
                let n = stream.read(&buffer, maxLength: buffer.count)
                if n <= 0 { break }
                out.append(buffer, count: n)
            }
            data = out
        }
        return try XCTUnwrap(JSONSerialization.jsonObject(with: XCTUnwrap(data)) as? [String: Any])
    }

    func testPutDeviceSendsTheContract() async throws {
        let registration = PushDeviceRegistration(token: token, environment: .production, appVersion: "1.2.0", os: "iOS 26.0")
        let request = try await withStub([.init(status: 200, body: "{}")]) {
            try await httpClient().putDevice(installationId: "abc-12345", registration: registration)
        }
        XCTAssertEqual(request?.httpMethod, "PUT")
        XCTAssertEqual(request?.url?.path, "/v1/me/devices/abc-12345")
        let body = try bodyJSON(request)
        XCTAssertEqual(Set(body.keys), ["token", "environment", "appVersion", "os"])
        XCTAssertEqual(body["token"] as? String, token)
        XCTAssertEqual(body["environment"] as? String, "production")
    }

    func testDeleteDevice() async throws {
        let request = try await withStub([.init(status: 204)]) {
            try await httpClient().deleteDevice(installationId: "abc-12345")
        }
        XCTAssertEqual(request?.httpMethod, "DELETE")
        XCTAssertEqual(request?.url?.path, "/v1/me/devices/abc-12345")
    }

    func testSetNotifyConsent() async throws {
        let request = try await withStub([.init(status: 200, body: "{}")]) {
            try await httpClient().setNotifyConsent(appId: "app_1", allowed: false)
        }
        XCTAssertEqual(request?.httpMethod, "PUT")
        XCTAssertEqual(request?.url?.path, "/v1/me/notify-consent/app_1")
        XCTAssertEqual(try bodyJSON(request)["allowed"] as? Bool, false)
    }

    func testNotMemberIsAnError() async {
        do {
            _ = try await withStub([.init(status: 403, body: #"{"error":{"code":"not_member","message":"no"}}"#)]) {
                try await httpClient().setNotifyConsent(appId: "app_1", allowed: true)
            }
            XCTFail("throw するはず")
        } catch {
            XCTAssertEqual((error as? APIError)?.status, 403)
        }
    }

    // MARK: - 読み方

    func testPayloadAppId() {
        XCTAssertEqual(PushPayload.appId(from: ["appId": "app_01:x.y-z", "aps": ["alert": ["title": "t"]]]), "app_01:x.y-z")
        XCTAssertNil(PushPayload.appId(from: ["aps": [:]]), "ローカル通知")
        XCTAssertNil(PushPayload.appId(from: ["appId": 1]))
        XCTAssertNil(PushPayload.appId(from: ["appId": ""]))
        XCTAssertNil(PushPayload.appId(from: ["appId": "https://evil.example/"]))
    }

    func testTokenHexIsLowercase() {
        XCTAssertEqual(PushPayload.tokenHex(Data([0x00, 0xAB, 0xFF])), "00abff")
    }

    func testEnvironmentFromProvisioningProfile() {
        let profile = { (env: String) in Data("<key>aps-environment</key>\n\t\t<string>\(env)</string>".utf8) }
        XCTAssertEqual(PushEnvironment.from(provisioningProfile: profile("development")), .sandbox)
        XCTAssertEqual(PushEnvironment.from(provisioningProfile: profile("production")), .production)
        XCTAssertEqual(PushEnvironment.from(provisioningProfile: Data("no entitlement".utf8)), .production)
        // Mac の profile のキー(#982)
        XCTAssertEqual(PushEnvironment.from(provisioningProfile: Data("<key>com.apple.developer.aps-environment</key><string>development</string>".utf8)), .sandbox)
        XCTAssertEqual(PushEnvironment.from(provisioningProfile: nil), .production)
    }

    func testConsentStoreIsPerUserAndApp() {
        let store = NotifyConsentStore(defaults: defaults)
        XCTAssertFalse(store.allowed(uid: "u1", appId: "a"), "保存が無い = 受け取らない")
        store.set(true, uid: "u1", appId: "a")
        XCTAssertTrue(store.allowed(uid: "u1", appId: "a"))
        XCTAssertFalse(store.allowed(uid: "u2", appId: "a"))
        XCTAssertFalse(store.allowed(uid: "u1", appId: "b"))
    }
}
