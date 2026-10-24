import XCTest
import MetamonDSL
@testable import MetamonServices

final class MockServicesTests: XCTestCase {
    func testInMemoryAppConfigCRUD() async throws {
        let client = InMemoryAppConfigClient(seed: MockSeed.demoRecords())
        var rows = try await client.list(appId: "a", entity: "expense")
        XCTAssertEqual(rows.count, 2)

        let created = try await client.create(appId: "a", entity: "expense", data: ["amount": 100])
        rows = try await client.list(appId: "a", entity: "expense")
        XCTAssertEqual(rows.count, 3)

        let updated = try await client.update(appId: "a", entity: "expense", id: created.id, data: ["amount": 200])
        XCTAssertEqual(updated["amount"], 200)

        try await client.delete(appId: "a", entity: "expense", id: created.id)
        rows = try await client.list(appId: "a", entity: "expense")
        XCTAssertEqual(rows.count, 2)

        do {
            _ = try await client.update(appId: "a", entity: "expense", id: "nope", data: [:])
            XCTFail("expected 404")
        } catch APIError.http(let status, _) {
            XCTAssertEqual(status, 404)
        }
    }

    func testMockAIClientRouting() {
        XCTAssertEqual(MockAIClient.sampleName(for: "筋トレを記録したい"), "training")
        XCTAssertEqual(MockAIClient.sampleName(for: "散歩の写真を残したい"), "walk")
        XCTAssertEqual(MockAIClient.sampleName(for: "最初に戻して"), "initial")
        XCTAssertEqual(MockAIClient.sampleName(for: "家計簿"), "kakeibo")
    }

    func testMockAIClientKeepsAppIdAndSummary() async throws {
        // サンプル DSL はアプリ本体のバンドル(docs/dsl/examples)にしか無い。テストホストに無ければスキップ
        guard Bundle.main.url(forResource: "kakeibo", withExtension: "json") != nil else {
            throw XCTSkip("DSL examples are not bundled in the test host")
        }
        let r = try await MockAIClient(delay: .zero).morph(transcript: "家計簿", appId: "device-1", currentVersion: 3)
        XCTAssertEqual(r.document.appId, "device-1")
        XCTAssertEqual(r.document.version, 4)
        XCTAssertNotNil(r.summary)
    }

    @MainActor func testMockTranscriber() async throws {
        let t = MockTranscriber(cannedText: "テスト", delay: .zero)
        XCTAssertTrue(t.isReady)
        XCTAssertEqual(t.status, .ready)
        try await t.startRecording()
        let text = try await t.stopAndTranscribe()
        XCTAssertEqual(text, "テスト")
    }

    func testNotificationTimeParsing() {
        XCTAssertEqual(UserNotificationScheduler.parseTime("08:30")?.0, 8)
        XCTAssertEqual(UserNotificationScheduler.parseTime("08:30")?.1, 30)
        XCTAssertNil(UserNotificationScheduler.parseTime("25:00"))
        XCTAssertNil(UserNotificationScheduler.parseTime(nil))
        XCTAssertEqual(UserNotificationScheduler.weekdayNumber("sun"), 1)
        XCTAssertEqual(UserNotificationScheduler.weekdayNumber(nil), 2)
    }

    func testWhisperVariantResolution() {
        XCTAssertEqual(WhisperKitRecognizer.resolveVariant("small"), "openai_whisper-small")
        XCTAssertEqual(WhisperKitRecognizer.resolveVariant("openai_whisper-large-v3"), "openai_whisper-large-v3")
        XCTAssertEqual(WhisperKitRecognizer.resolveVariant("distil-whisper_distil-large-v3"), "distil-whisper_distil-large-v3")
    }

    // MARK: - Auth

    func testMockAuthLifecycle() async throws {
        let auth = MockAuthService(signedIn: false)
        XCTAssertNil(auth.currentUser)
        let anon = try await auth.signInAnonymously()
        XCTAssertTrue(anon.isAnonymous)
        XCTAssertEqual(auth.currentUser?.uid, MockAuthService.fixedUID)
        let token = try await AuthTokenProvider(auth: auth).idToken()
        XCTAssertNil(token, "モックは Authorization を付けない")

        var name = PersonNameComponents()
        name.givenName = "太郎"
        let apple = try await auth.signInWithApple(idToken: "t", nonce: "n", fullName: name)
        XCTAssertFalse(apple.isAnonymous)
        XCTAssertNotNil(apple.displayName)

        try auth.signOut()
        XCTAssertNil(auth.currentUser)
    }

    func testMockAuthStateStreamEmitsCurrentThenChanges() async throws {
        let auth = MockAuthService(signedIn: false)
        var it = auth.stateChanges().makeAsyncIterator()
        let first = await it.next()
        XCTAssertEqual(first, .some(nil))
        _ = try await auth.signInAnonymously()
        let second = await it.next()
        XCTAssertEqual(second??.uid, MockAuthService.fixedUID)
    }

    // MARK: - ai 契約

    func testHTTPAIClientParsesGenerateResponse() throws {
        let json = """
        {"appId":"dev-1","version":2,"previousVersion":1,"mode":"full",
         "dsl":{"schemaVersion":1,"app":{"name":"家計簿","voiceEntry":"floating"},"initialScreen":"home","entities":[],
                "screens":[{"id":"home","title":"ホーム","components":[{"type":"text","text":"hi"}]}]},
         "summary":"家計簿アプリを作成しました","transcript":"家計簿","source":"claude",
         "latencyMs":{"claude":1200,"total":1300},"meta":{"model":"m","attempts":1,"regenerations":0,"areaFallback":false,"usage":{}}}
        """
        let r = try HTTPAIClient.parse(Data(json.utf8), appId: "fallback")
        XCTAssertEqual(r.document.appId, "dev-1")
        XCTAssertEqual(r.document.version, 2)
        XCTAssertEqual(r.document.dsl.app.name, "家計簿")
        XCTAssertEqual(r.summary, "家計簿アプリを作成しました")
        XCTAssertEqual(r.mode, "full")
    }

    func testGenerateBodyShape() throws {
        let body = HTTPAIClient.GenerateBody(appId: "a", baseVersion: nil, transcript: "x")
        let obj = try JSONSerialization.jsonObject(with: JSONEncoder().encode(body)) as? [String: Any]
        XCTAssertEqual(obj?["appId"] as? String, "a")
        XCTAssertEqual(obj?["mode"] as? String, "auto")
        XCTAssertNil(obj?["baseVersion"], "未指定なら省略(サーバーは現行 version を使う)")
    }

    func testErrorEnvelopeDecodes() throws {
        let data = Data(#"{"error":{"code":"generation_failed","message":"could not produce"}}"#.utf8)
        let env = try JSONDecoder().decode(HTTP.ErrorEnvelope.self, from: data)
        XCTAssertEqual(env.error.code, "generation_failed")
        XCTAssertEqual(APIError.server(status: 422, code: env.error.code, message: env.error.message).errorDescription, "could not produce")
    }
}
