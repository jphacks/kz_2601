import XCTest
import MetamonServices
import MetamonRenderer
@testable import Metamon

/// 話して入力(`DictationHost`)を `SpeechTranscriber` につなぐ共有の部品(#1328・Mac の MacAppRuntime が使う)。
/// 認識は偽物に差し替え、マイク・音声モデル・AI は使わない。
@MainActor
final class TranscriberDictationTests: XCTestCase {
    private final class FakeTranscriber: SpeechTranscriber {
        var isReady = true
        var status: TranscriberStatus = .ready
        var startError: Error?
        var transcript = "こんにちは"
        var started = 0, stopped = 0, cancelled = 0, prepared = 0

        func prepare() async throws { prepared += 1 }
        func startRecording() async throws {
            if let startError { throw startError }
            started += 1
        }
        func stopAndTranscribe() async throws -> String {
            stopped += 1
            return transcript
        }
        func cancelRecording() { cancelled += 1 }
    }

    private var speechLog: [String] = []
    private var modelRequests = 0
    private var voiceActive = false

    private func make(_ t: FakeTranscriber) -> TranscriberDictation {
        TranscriberDictation(
            transcriber: t,
            voiceInputActive: { [unowned self] in voiceActive },
            stopSpeech: { [unowned self] in speechLog.append("stop") },
            resumeSpeech: { [unowned self] in speechLog.append("resume") },
            failure: { error in
                let pane = (error as? TranscriberFailure) == .microphoneDenied
                    ? URL(string: "x-apple.systempreferences:com.apple.preference.security?Privacy_Microphone") : nil
                return DictationFailure("失敗", settingsURL: pane)
            },
            requestModel: { [unowned self] in modelRequests += 1 })
    }

    func testStartAndFinishStopsSpeechAndReturnsText() async throws {
        let t = FakeTranscriber()
        let d = make(t)
        try await d.start()
        XCTAssertTrue(d.isRecording)
        XCTAssertEqual(speechLog, ["stop"], "録音を始める前に読み上げを止める")
        let text = try await d.finish()
        XCTAssertEqual(text, "こんにちは")
        XCTAssertFalse(d.isRecording)
        XCTAssertEqual(speechLog, ["stop", "resume"])
        XCTAssertEqual(t.started, 1)
        XCTAssertEqual(t.stopped, 1)
    }

    func testDeniedMicrophoneCarriesSettingsURL() async {
        let t = FakeTranscriber()
        t.startError = TranscriberFailure.microphoneDenied
        let d = make(t)
        do {
            try await d.start()
            XCTFail("始められないはず")
        } catch let f as DictationFailure {
            XCTAssertEqual(f.settingsURL?.absoluteString,
                           "x-apple.systempreferences:com.apple.preference.security?Privacy_Microphone")
        } catch {
            XCTFail("DictationFailure で返す: \(error)")
        }
        XCTAssertFalse(d.isRecording)
        XCTAssertEqual(speechLog, ["stop", "resume"], "失敗したら読み上げの抑止を外す")
    }

    func testEmptyTranscriptIsAFailure() async throws {
        let t = FakeTranscriber()
        t.transcript = "  \n"
        let d = make(t)
        try await d.start()
        do {
            _ = try await d.finish()
            XCTFail("空は失敗")
        } catch is DictationFailure {}
    }

    func testCancelOnlyDropsOwnRecording() async throws {
        let t = FakeTranscriber()
        let d = make(t)
        d.cancel()
        XCTAssertEqual(t.cancelled, 0, "部品が録音していなければ、声で頼むの録音を止めない")
        try await d.start()
        d.cancel()
        XCTAssertEqual(t.cancelled, 1)
        XCTAssertFalse(d.isRecording)
        d.cancel()
        XCTAssertEqual(t.cancelled, 1)
    }

    func testPrepareAsksForModelUnlessFailed() async {
        let t = FakeTranscriber()
        t.isReady = false
        t.status = .notDownloaded
        let d = make(t)
        XCTAssertEqual(d.readiness, .needsModel)
        d.prepare()
        XCTAssertEqual(modelRequests, 1, "モデルが無い時はホストの確認へ")
        XCTAssertEqual(t.prepared, 0)

        t.status = .failed("x")
        XCTAssertEqual(d.readiness, .failed)
        d.prepare()
        XCTAssertEqual(modelRequests, 1, "失敗後は確認せず作り直す")
    }

    func testVoiceInputActiveComesFromHost() {
        let d = make(FakeTranscriber())
        XCTAssertFalse(d.isVoiceInputActive)
        voiceActive = true
        XCTAssertTrue(d.isVoiceInputActive)
    }
}
