import XCTest
import MetamonDSL
@testable import MetamonRenderer
import MetamonServices
@testable import Metamon

/// 録音ファイルの文字起こし(`AudioFileTranscriptionHost`)と、録音と再生は同時に 1 つだけ(`AudioFocus`)の約束(#952)
@MainActor
final class MacAudioFocusTests: XCTestCase {
    // MARK: - 偽の認識

    private final class FakeRecognizer: SpeechTranscriber, AudioFileTranscriber {
        var isReady = true
        var status: TranscriberStatus = .ready
        var canTranscribeFiles = true
        var result: Result<String, Error> = .success("こんにちは")
        private(set) var transcribed: [URL] = []
        private(set) var recording = false

        func prepare() async throws {}
        func startRecording() async throws { recording = true }
        func stopAndTranscribe() async throws -> String { recording = false; return "声" }
        func cancelRecording() { recording = false }

        func transcribeFile(at url: URL, progress: @escaping @MainActor (Double) -> Void) async throws -> String {
            transcribed.append(url)
            progress(0.5)
            progress(1)
            return try result.get()
        }
    }

    private let file = URL(fileURLWithPath: "/tmp/mac952.m4a")

    private func kakeibo() throws -> AppDSL {
        let url = try XCTUnwrap(Bundle(for: MacAppRuntime.self).url(forResource: "kakeibo", withExtension: "json"))
        return try AppDSL.load(contentsOf: url)
    }

    private func runtime(_ recognizer: FakeRecognizer?, voiceActive: @escaping () -> Bool = { false },
                         focus: AudioFocus? = nil) throws -> MacAppRuntime {
        let dictation = recognizer.map {
            TranscriberDictation(transcriber: $0, voiceInputActive: voiceActive, stopSpeech: {}, resumeSpeech: {},
                                 failure: { MacVoiceCopy.dictationFailure($0) }, requestModel: {})
        }
        let rt = MacAppRuntime(appId: "test-mac952-\(UUID().uuidString.lowercased())", dsl: try kakeibo(),
                               ai: MockAIClient(delay: .zero), isAIConsentGranted: { false }, dictation: dictation)
        rt.audioFocus = focus ?? AudioFocus(stopPlayback: {})
        return rt
    }

    // MARK: - 録音ファイルの文字起こし

    func testRuntimeIsAnAudioFileTranscriptionHost() throws {
        let rt = try runtime(FakeRecognizer())
        XCTAssertNotNil(rt.audioFileTranscription)
    }

    func testTranscribesFileWithTheRecognizerAndReportsProgress() async throws {
        let fake = FakeRecognizer()
        let rt = try runtime(fake)
        var steps: [Double] = []
        let text = try await rt.transcribeAudioFile(file) { steps.append($0) }
        XCTAssertEqual(text, "こんにちは")
        XCTAssertEqual(fake.transcribed, [file])
        XCTAssertEqual(steps, [0.5, 1])
    }

    func testReadinessMirrorsIOS() {
        XCTAssertEqual(MacAppRuntime.audioFileReadiness(canTranscribe: true, status: .ready), .ready)
        XCTAssertEqual(MacAppRuntime.audioFileReadiness(canTranscribe: false, status: .notDownloaded), .needsModel)
        XCTAssertEqual(MacAppRuntime.audioFileReadiness(canTranscribe: false, status: .failed("x")), .failed)
        XCTAssertEqual(MacAppRuntime.audioFileReadiness(canTranscribe: false, status: .ready), .preparing) // 録音・別の文字起こし中
        XCTAssertEqual(MacAppRuntime.audioFileReadiness(canTranscribe: false, status: .downloading), .preparing)
    }

    func testNoRecognizerIsFailed() throws {
        XCTAssertEqual(try runtime(nil).audioFileReadiness, .failed)
    }

    func testReadinessIsPreparingWhileAudioRecorderRecords() throws {
        let focus = AudioFocus(stopPlayback: {})
        let rt = try runtime(FakeRecognizer(), focus: focus)
        XCTAssertEqual(rt.audioFileReadiness, .ready)
        let lease = try AudioSessionLease.forRecording(focus)
        XCTAssertEqual(rt.audioFileReadiness, .preparing)
        lease.end()
        XCTAssertEqual(rt.audioFileReadiness, .ready)
    }

    func testRefusesWhileVoiceInputIsActive() async throws {
        let fake = FakeRecognizer()
        let rt = try runtime(fake, voiceActive: { true })
        do {
            _ = try await rt.transcribeAudioFile(file) { _ in }
            XCTFail("声の入力中は文字にしない")
        } catch let f as DictationFailure {
            XCTAssertEqual(f.message, L10n.tr("音声入力を使っている間は、録音ファイルを文字にできません"))
        }
        XCTAssertTrue(fake.transcribed.isEmpty)
    }

    func testRefusesWhileAudioRecorderRecords() async throws {
        let focus = AudioFocus(stopPlayback: {})
        let fake = FakeRecognizer()
        let rt = try runtime(fake, focus: focus)
        let lease = try AudioSessionLease.forRecording(focus)
        defer { lease.end() }
        do {
            _ = try await rt.transcribeAudioFile(file) { _ in }
            XCTFail("録音中は文字にしない")
        } catch let f as DictationFailure {
            XCTAssertEqual(f.message, MacVoiceCopy.otherRecording)
        }
        XCTAssertTrue(fake.transcribed.isEmpty)
    }

    func testTranscriberFailureUsesMacWording() async throws {
        let fake = FakeRecognizer()
        fake.result = .failure(TranscriberFailure.notReady("loading"))
        let rt = try runtime(fake)
        do {
            _ = try await rt.transcribeAudioFile(file) { _ in }
            XCTFail()
        } catch let f as DictationFailure {
            XCTAssertEqual(f.message, MacVoiceCopy.message(.notReady("loading")))
        }
    }

    func testCancellationPassesThrough() async throws {
        let fake = FakeRecognizer()
        fake.result = .failure(CancellationError())
        let rt = try runtime(fake)
        do {
            _ = try await rt.transcribeAudioFile(file) { _ in }
            XCTFail()
        } catch is CancellationError {}
    }

    func testOtherErrorsBecomeAGenericFailure() async throws {
        let fake = FakeRecognizer()
        fake.result = .failure(CocoaError(.fileReadCorruptFile))
        let rt = try runtime(fake)
        do {
            _ = try await rt.transcribeAudioFile(file) { _ in }
            XCTFail()
        } catch let f as DictationFailure {
            XCTAssertEqual(f.message, L10n.tr("録音ファイルを文字にできませんでした。もう一度お試しください"))
        }
    }

    // MARK: - 録音と再生は同時に 1 つだけ

    func testRecordingStopsPlaybackAndSpeech() throws {
        var stops = 0
        var hostStops = 0
        let focus = AudioFocus(stopPlayback: { stops += 1 })
        focus.stopHostPlayback = { hostStops += 1 }
        let playback = try AudioSessionLease.forPlayback(focus)
        XCTAssertTrue(focus.isPlaying)
        let recording = try AudioSessionLease.forRecording(focus)
        XCTAssertEqual(stops, 1)
        XCTAssertEqual(hostStops, 1)
        XCTAssertFalse(focus.isPlaying)
        XCTAssertTrue(focus.isRecording)
        playback.end() // 止まった再生の後片付けは何もしない
        XCTAssertTrue(focus.isRecording)
        recording.end()
        recording.end() // 何度返してもよい
        XCTAssertFalse(focus.isRecording)
    }

    func testPlaybackIsRefusedWhileRecording() throws {
        let focus = AudioFocus(stopPlayback: {})
        let recording = try AudioSessionLease.forRecording(focus)
        XCTAssertThrowsError(try AudioSessionLease.forPlayback(focus)) {
            XCTAssertEqual($0 as? AudioFocus.Refusal, .recordingInProgress)
        }
        recording.end()
        XCTAssertNoThrow(try AudioSessionLease.forPlayback(focus).end())
    }

    func testPlaybackIsRefusedWhileVoiceInputRecords() {
        let focus = AudioFocus(stopPlayback: {})
        var voiceActive = true
        focus.isMicBusyElsewhere = { voiceActive }
        XCTAssertThrowsError(try AudioSessionLease.forPlayback(focus))
        XCTAssertThrowsError(try AudioSessionLease.forRecording(focus)) {
            XCTAssertEqual($0 as? AudioFocus.Refusal, .micInUse)
        }
        voiceActive = false
        XCTAssertNoThrow(try AudioSessionLease.forPlayback(focus).end())
    }

    func testSecondRecordingIsRefused() throws {
        let focus = AudioFocus(stopPlayback: {})
        let first = try AudioSessionLease.forRecording(focus)
        XCTAssertThrowsError(try AudioSessionLease.forRecording(focus))
        first.end()
        XCTAssertNoThrow(try AudioSessionLease.forRecording(focus).end())
    }

    func testPlaybackCanReplacePlayback() throws {
        let focus = AudioFocus(stopPlayback: {})
        let a = try AudioSessionLease.forPlayback(focus)
        let b = try AudioSessionLease.forPlayback(focus)
        a.end()
        XCTAssertTrue(focus.isPlaying)
        b.end()
        XCTAssertFalse(focus.isPlaying)
    }

    func testVoiceInputStartSilencesPlayback() throws {
        var stops = 0
        let focus = AudioFocus(stopPlayback: { stops += 1 })
        _ = try AudioSessionLease.forPlayback(focus)
        focus.micWillStart()
        XCTAssertEqual(stops, 1)
        XCTAssertFalse(focus.isPlaying)
    }

    func testVoiceInputSharesStateWithFocus() {
        let focus = AudioFocus(stopPlayback: {})
        let voice = MacVoiceInput(transcriber: FakeRecognizer(), defaults: UserDefaults(suiteName: "mac952-\(UUID())")!,
                                  slowHardware: false, isExpensiveNetwork: { false }, currentInputUID: { nil },
                                  audioFocus: focus)
        voice.attachAudioFocus()
        XCTAssertFalse(focus.isRecordingAnywhere)
        voice.toggle { _ in }
        XCTAssertTrue(voice.isBusy)
        XCTAssertTrue(focus.isRecordingAnywhere) // 声で頼んでいる間は再生しない
        XCTAssertThrowsError(try AudioSessionLease.forPlayback(focus))
        voice.cancel()
        XCTAssertFalse(focus.isRecordingAnywhere)
    }

    func testVoiceInputIsRefusedWhileAudioRecorderRecords() throws {
        let focus = AudioFocus(stopPlayback: {})
        let voice = MacVoiceInput(transcriber: FakeRecognizer(), defaults: UserDefaults(suiteName: "mac952-\(UUID())")!,
                                  slowHardware: false, isExpensiveNetwork: { false }, currentInputUID: { nil },
                                  audioFocus: focus)
        let recording = try AudioSessionLease.forRecording(focus)
        defer { recording.end() }
        voice.toggle { _ in }
        XCTAssertFalse(voice.isBusy)
        XCTAssertEqual(voice.failure, MacVoiceCopy.otherRecording)
    }

    func testDictationIsRefusedWhileAudioRecorderRecords() async throws {
        let focus = AudioFocus(stopPlayback: {})
        let fake = FakeRecognizer()
        let rt = try runtime(fake, focus: focus)
        let recording = try AudioSessionLease.forRecording(focus)
        defer { recording.end() }
        do {
            try await rt.startDictation()
            XCTFail()
        } catch let f as DictationFailure {
            XCTAssertEqual(f.message, MacVoiceCopy.otherRecording)
        }
        XCTAssertFalse(fake.recording)
    }

    // MARK: - マイクの許可が無い時の案内

    func testMicrophoneDeniedPointsToSystemSettingsMicrophone() {
        XCTAssertEqual(PlatformSystem.microphoneSettingsURL?.absoluteString,
                       "x-apple.systempreferences:com.apple.preference.security?Privacy_Microphone")
        let message = PlatformSystem.microphoneDeniedMessage
        XCTAssertFalse(message.contains("設定アプリ") || message.contains("Settings app"), message) // iPhone の言い方にしない
        XCTAssertEqual(PlatformSystem.openSettingsTitle, L10n.tr("システム設定を開く"))
        XCTAssertEqual(MacVoiceCopy.privacyPane(for: .microphoneDenied)?.url.absoluteString,
                       "x-apple.systempreferences:com.apple.preference.security?Privacy_Microphone")
    }
}
