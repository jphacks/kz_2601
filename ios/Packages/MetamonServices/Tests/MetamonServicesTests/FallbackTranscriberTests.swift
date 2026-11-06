import XCTest
@testable import MetamonServices

/// U5: WhisperKit の準備中は Apple の端末内の認識で仮に聞き取り、準備ができたら WhisperKit に替える。
@MainActor
final class FallbackTranscriberTests: XCTestCase {
    /// 本命(WhisperKit)の偽物
    final class FakePrimary: SpeechTranscriber {
        var isReady = false
        var status: TranscriberStatus = .loading
        var text = "whisper"
        var started = 0
        var stopped = 0
        var cancelled = 0
        var prepared = 0
        func prepare() async throws { prepared += 1 }
        func startRecording() async throws {
            guard isReady else { throw TranscriberFailure.notReady("\(status)") }
            started += 1
        }
        func stopAndTranscribe() async throws -> String { stopped += 1; return text }
        func cancelRecording() { cancelled += 1 }
    }

    /// 仮の聞き取りの偽物
    final class FakeProvisional: ProvisionalTranscriber {
        var isAvailable = true
        var isReady: Bool { isAvailable }
        var startError: Error?
        var started = 0
        var stopped = 0
        var cancelled = 0
        func prepare() async throws {}
        func startRecording() async throws {
            if let startError { throw startError }
            started += 1
        }
        func stopAndTranscribe() async throws -> String { stopped += 1; return "apple" }
        func cancelRecording() { cancelled += 1 }
    }

    func testUsesAppleWhileWhisperIsPreparing() async throws {
        let primary = FakePrimary(), apple = FakeProvisional()
        let t = FallbackTranscriber(primary: primary, provisional: apple)
        XCTAssertTrue(t.isReady, "準備中でも仮の聞き取りでマイクを押せる")
        XCTAssertTrue(t.isProvisional)
        XCTAssertEqual(t.status, .loading, "モデルの状態は本命のまま見せる")

        try await t.startRecording()
        let text = try await t.stopAndTranscribe()
        XCTAssertEqual(text, "apple")
        XCTAssertEqual(apple.started, 1)
        XCTAssertEqual(primary.started, 0)
    }

    func testUsesWhisperOnceReady() async throws {
        let primary = FakePrimary(), apple = FakeProvisional()
        primary.isReady = true
        primary.status = .ready
        let t = FallbackTranscriber(primary: primary, provisional: apple)
        XCTAssertFalse(t.isProvisional)

        try await t.startRecording()
        let text = try await t.stopAndTranscribe()
        XCTAssertEqual(text, "whisper")
        XCTAssertEqual(apple.started, 0)
    }

    func testSwitchesOnlyBetweenRecordings() async throws {
        let primary = FakePrimary(), apple = FakeProvisional()
        let t = FallbackTranscriber(primary: primary, provisional: apple)

        try await t.startRecording() // 仮で始める
        primary.isReady = true       // 録音中に本命の準備が終わる
        XCTAssertTrue(t.isProvisional, "録音中は始めた方のまま")
        let first = try await t.stopAndTranscribe()
        XCTAssertEqual(first, "apple", "録音は始めた方で止める")
        XCTAssertEqual(primary.stopped, 0)

        XCTAssertFalse(t.isProvisional, "次の録音から本命")
        try await t.startRecording()
        let second = try await t.stopAndTranscribe()
        XCTAssertEqual(second, "whisper")
    }

    func testCancelGoesToActiveEngine() async throws {
        let primary = FakePrimary(), apple = FakeProvisional()
        let t = FallbackTranscriber(primary: primary, provisional: apple)
        try await t.startRecording()
        t.cancelRecording()
        XCTAssertEqual(apple.cancelled, 1)
        XCTAssertEqual(primary.cancelled, 0)
        let text = try await t.stopAndTranscribe()
        XCTAssertEqual(text, "", "止めた後は何も起きない")
    }

    func testNotUsedWhenOnDeviceUnavailable() async throws {
        let primary = FakePrimary(), apple = FakeProvisional()
        apple.isAvailable = false
        let t = FallbackTranscriber(primary: primary, provisional: apple)
        XCTAssertFalse(t.isReady, "端末内が使えなければ今まで通り灰色(文字を先に)")
        XCTAssertFalse(t.isProvisional)
        do {
            try await t.startRecording()
            XCTFail("準備中は本命の notReady のまま")
        } catch let f as TranscriberFailure {
            guard case .notReady = f else { return XCTFail("\(f)") }
        }
        XCTAssertEqual(apple.started, 0)
    }

    func testPrepareAndModelStateForwardToPrimary() async throws {
        let primary = FakePrimary(), apple = FakeProvisional()
        primary.status = .downloading
        let t = FallbackTranscriber(primary: primary, provisional: apple)
        try await t.prepare()
        XCTAssertEqual(primary.prepared, 1)
        XCTAssertEqual(t.status, .downloading)
    }

    func testDeniedStartLeavesIdle() async throws {
        let primary = FakePrimary(), apple = FakeProvisional()
        apple.startError = TranscriberFailure.speechRecognitionDenied
        let t = FallbackTranscriber(primary: primary, provisional: apple)
        do {
            try await t.startRecording()
            XCTFail("許可が無ければ始まらない")
        } catch let f as TranscriberFailure {
            XCTAssertEqual(f, .speechRecognitionDenied)
        }
        XCTAssertNil(t.active, "失敗したら録音中にしない")
    }

    // MARK: - AppleOnDeviceTranscriber の録音前の確認

    final class Counter { var value = 0 }

    private func support(auth: SpeechAuthorization, afterRequest: SpeechAuthorization = .authorized,
                         onDevice: Bool = true, requests: Counter? = nil) -> OnDeviceSpeechSupport {
        OnDeviceSpeechSupport(
            authorization: { auth },
            requestAuthorization: { requests?.value += 1; return afterRequest },
            supportsOnDevice: { _ in onDevice }
        )
    }

    func testAppleNotAvailableWithoutOnDeviceSupport() async {
        let t = AppleOnDeviceTranscriber(support: support(auth: .authorized, onDevice: false))
        XCTAssertFalse(t.isAvailable, "サーバーへ送る認識には落とさない")
        do {
            _ = try await t.checkBeforeRecording()
            XCTFail()
        } catch {
            XCTAssertEqual(error as? TranscriberFailure, .onDeviceRecognitionUnavailable)
        }
    }

    func testAppleNotAvailableWhenDenied() async {
        for status in [SpeechAuthorization.denied, .restricted] {
            let t = AppleOnDeviceTranscriber(support: support(auth: status))
            XCTAssertFalse(t.isAvailable)
            do {
                _ = try await t.checkBeforeRecording()
                XCTFail()
            } catch {
                XCTAssertEqual(error as? TranscriberFailure, .speechRecognitionDenied)
            }
        }
    }

    func testAppleAsksPermissionOnFirstUse() async throws {
        let requests = Counter()
        let granted = AppleOnDeviceTranscriber(support: support(auth: .notDetermined, afterRequest: .authorized, requests: requests))
        XCTAssertTrue(granted.isAvailable, "未決定なら押せる(最初の録音で聞く)")
        let lang = try await granted.checkBeforeRecording()
        XCTAssertEqual(lang, "ja")
        XCTAssertEqual(requests.value, 1)
        XCTAssertEqual(granted.authorization, .authorized)

        let refused = AppleOnDeviceTranscriber(support: support(auth: .notDetermined, afterRequest: .denied))
        do {
            _ = try await refused.checkBeforeRecording()
            XCTFail()
        } catch {
            XCTAssertEqual(error as? TranscriberFailure, .speechRecognitionDenied)
        }
        XCTAssertFalse(refused.isAvailable, "断られたら以後は灰色(文字を先に)")
    }

    func testAppleLocaleAndPrecheck() {
        XCTAssertEqual(AppleOnDeviceTranscriber.localeID(for: "ja"), "ja-JP")
        XCTAssertEqual(AppleOnDeviceTranscriber.localeID(for: "en"), "en-US")
        XCTAssertEqual(AppleOnDeviceTranscriber.precheck(frames: 0, seconds: 1, peak: 0), .noAudioCaptured(seconds: 1))
        XCTAssertEqual(AppleOnDeviceTranscriber.precheck(frames: 4800, seconds: 0.1, peak: 0.3), .tooShort(seconds: 0.1))
        XCTAssertNil(AppleOnDeviceTranscriber.precheck(frames: 48000, seconds: 1, peak: 0.3))
    }
}
