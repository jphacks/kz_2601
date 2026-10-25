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

    /// 0 サンプル / 短すぎ / 無音は "" ではなく理由付きの TranscriberFailure になる(toast にそのまま出す)。
    func testWhisperPrecheckExplainsEmptyRecordings() {
        XCTAssertEqual(WhisperKitRecognizer.precheck(sampleCount: 0, seconds: 2.0, peak: 0), .noAudioCaptured(seconds: 2.0))
        XCTAssertEqual(WhisperKitRecognizer.precheck(sampleCount: 3200, seconds: 0.2, peak: 0.3), .tooShort(seconds: 0.2))
        guard case .silent(let seconds, let db)? = WhisperKitRecognizer.precheck(sampleCount: 32000, seconds: 2.0, peak: 0.0001) else {
            return XCTFail("expected .silent")
        }
        XCTAssertEqual(seconds, 2.0)
        XCTAssertEqual(db, -80, accuracy: 0.01)
        XCTAssertNil(WhisperKitRecognizer.precheck(sampleCount: 32000, seconds: 2.0, peak: 0.2))

        let message = WhisperKitRecognizer.precheck(sampleCount: 0, seconds: 1.5, peak: 0)?.errorDescription ?? ""
        XCTAssertTrue(message.contains("0サンプル"), message)
        XCTAssertTrue(message.contains("1.5秒"), message)
    }

    func testWhisperLevels() {
        let empty = WhisperKitRecognizer.levels(of: [])
        XCTAssertEqual(empty.0, 0)
        XCTAssertEqual(empty.1, 0)
        let (peak, rms) = WhisperKitRecognizer.levels(of: [0.5, -0.25, 0])
        XCTAssertEqual(peak, 0.5, accuracy: 1e-6)
        XCTAssertEqual(rms, 0.3227, accuracy: 1e-3)
    }

    @MainActor func testMockTranscriberHasNoDiagnostics() {
        XCTAssertNil(MockTranscriber().lastDiagnostics)
        var diag = TranscriberDiagnostics()
        diag.recordedSeconds = 2.3
        diag.peakAmplitude = 0.04
        diag.inputRoute = "iPhone Microphone"
        XCTAssertEqual(diag.summary, "録音2.3秒・音量-28dB・入力:iPhone Microphone")
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
        XCTAssertFalse(r.unchanged) // 省略 = 変化あり(旧サーバー)
    }

    /// #211: 変化なしの生成は unchanged=true と理由入りの summary
    func testHTTPAIClientParsesUnchanged() throws {
        let json = """
        {"appId":"dev-1","version":2,"previousVersion":2,"mode":"patch",
         "dsl":{"schemaVersion":1,"app":{"name":"家計簿","voiceEntry":"floating"},"initialScreen":"home","entities":[],
                "screens":[{"id":"home","title":"ホーム","components":[{"type":"text","text":"hi"}]}]},
         "summary":"項目は変更できませんでした。理由: 何の項目か決まっていないため。","locale":"ja","source":"claude","unchanged":true}
        """
        let r = try HTTPAIClient.parse(Data(json.utf8), appId: "fallback")
        XCTAssertTrue(r.unchanged)
        XCTAssertEqual(r.document.version, 2)
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

    // MARK: - appconfig 契約(PR #32)

    func testInvalidRecordDetailsExposeField() throws {
        let data = Data(#"{"error":{"code":"invalid_record","message":"field \"amount\" must be an integer","details":{"field":"amount"},"requestId":"r1"}}"#.utf8)
        let err = HTTP.error(from: HTTP.Response(status: 422, data: data, headers: [:]))
        XCTAssertEqual(err.status, 422)
        XCTAssertEqual(err.code, "invalid_record")
        XCTAssertEqual(err.field, "amount")
        XCTAssertEqual(err.errorDescription, "field \"amount\" must be an integer")
    }

    func testETagFormatMatchesBackend() {
        XCTAssertEqual(HTTPAppConfigClient.etag(7), "\"7\"")
    }

    func testVersionSummaryDecodes() throws {
        let json = """
        {"items":[{"appId":"a","version":3,"parentVersion":2,"source":{"kind":"restore","mode":"full","restoredFrom":1},"createdAt":"2026-10-24T10:00:00Z"},
                  {"appId":"a","version":2,"parentVersion":1,"source":{"kind":"ai","mode":"full","summary":"家計簿"},"createdAt":"2026-10-24T09:00:00Z"}]}
        """
        struct R: Decodable { var items: [VersionSummary] }
        let r = try JSONDecoder().decode(R.self, from: Data(json.utf8))
        XCTAssertEqual(r.items.map(\.version), [3, 2])
        XCTAssertEqual(r.items[0].source?.restoredFrom, 1)
        XCTAssertEqual(r.items[1].source?.summary, "家計簿")
    }

    func testAIEnvelopeForCache() throws {
        let data = Data(#"{"appId":"dev-1","version":2,"dsl":{"schemaVersion":1,"app":{"name":"x"},"initialScreen":"home","entities":[],"screens":[]},"summary":"s"}"#.utf8)
        let env = try XCTUnwrap(HTTPAIClient.envelope(from: data, appId: "dev-1", version: 2))
        let doc = try AppDocument.decode(env)
        XCTAssertEqual(doc.appId, "dev-1")
        XCTAssertEqual(doc.version, 2)
        XCTAssertEqual(doc.dsl.app.name, "x")
    }

    func testDocumentCacheRoundTrip() throws {
        let dir = FileManager.default.temporaryDirectory.appendingPathComponent("metamon-cache-\(UUID().uuidString)")
        let cache = DocumentCache(directory: dir)
        XCTAssertNil(cache.load(appId: "a"))
        let env = Data(#"{"appId":"a","version":4,"dsl":{"schemaVersion":1,"app":{"name":"家計簿"},"initialScreen":"home","entities":[],"screens":[]}}"#.utf8)
        cache.store(appId: "a", envelope: env)
        XCTAssertEqual(cache.load(appId: "a")?.version, 4)
        XCTAssertEqual(cache.load(appId: "a")?.dsl.app.name, "家計簿")
        cache.remove(appId: "a")
        XCTAssertNil(cache.load(appId: "a"))
    }

    // MARK: - 発話の意図(G11 / 新規)

    func testVoiceIntentUndo() {
        for t in ["元に戻して", "元に戻して。", "もとに戻して!", "取り消して", "アンドゥ", "戻して", "前に戻して", "ひとつ前の状態にして", "さっきの状態に戻して"] {
            XCTAssertEqual(VoiceIntent.detect(t), .undo, t)
        }
    }

    func testVoiceIntentNewApp() {
        for t in ["別のアプリ。家計簿が欲しい", "新しいアプリを作って、散歩メモ", "別アプリで筋トレ記録"] {
            XCTAssertEqual(VoiceIntent.detect(t), .newApp, t)
        }
    }

    func testVoiceIntentMorphIsDefault() {
        for t in ["家計簿が欲しい", "追加ボタンを左端まで広げて", "ボタンを右に戻して配置して", "", "   "] {
            XCTAssertEqual(VoiceIntent.detect(t), .morph, t)
        }
        // 「別のアプリ」が文中にあるだけでは新規扱いにしない(発話頭だけ)
        XCTAssertEqual(VoiceIntent.detect("家計簿に別のアプリへのリンクを足して"), .morph)
    }
}
