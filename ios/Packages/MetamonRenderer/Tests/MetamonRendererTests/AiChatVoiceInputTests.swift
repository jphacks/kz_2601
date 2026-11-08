#if !APP_STORE_REVIEW
import XCTest
@testable import MetamonRenderer

/// aiChat のマイク(話して入力): ボタンの状態・押せない時・録音と読み上げ・送信の兼ね合い。
@MainActor
final class AiChatVoiceInputTests: XCTestCase {
    private func state(_ phase: DictationPhase = .idle, _ readiness: DictationReadiness? = .ready,
                       fab: Bool = false, online: Bool = true) -> AiChatMic {
        AiChatMic.state(phase: phase, readiness: readiness, voiceInputActive: fab, online: online)
    }

    func testHiddenWithoutHost() {
        XCTAssertEqual(state(.idle, nil), .hidden, "話して入力を持たないホストではマイクを出さない")
    }

    func testIdleFollowsReadiness() {
        XCTAssertEqual(state(.idle, .ready), .start)
        XCTAssertEqual(state(.idle, .preparing), .busy, "モデルの準備中・録音ファイルの文字起こし中は押せない")
        XCTAssertEqual(state(.idle, .needsModel), .prepare)
        XCTAssertEqual(state(.idle, .failed), .prepare)
    }

    func testDisabledStates() {
        XCTAssertEqual(state(.idle, .ready, fab: true), .busy, "マイク FAB の音声入力とマイクを取り合わない")
        XCTAssertEqual(state(.idle, .ready, online: false), .busy, "AI と話せない版では押せない")
        XCTAssertFalse(AiChatMic.busy.enabled)
        XCTAssertFalse(AiChatMic.transcribing.enabled)
        XCTAssertTrue(AiChatMic.start.enabled)
        XCTAssertTrue(AiChatMic.stop.enabled)
        XCTAssertTrue(AiChatMic.prepare.enabled)
    }

    func testRecordingPhasesWinOverReadiness() {
        // 自分の録音を文字にしている間はモデルが recognizing(= preparing)でも「文字にしています」
        XCTAssertEqual(state(.listening, .preparing), .stop)
        XCTAssertEqual(state(.transcribing, .preparing), .transcribing)
        XCTAssertEqual(state(.listening, .ready, online: false), .stop, "録音中はいつでも止められる")
    }

    func testCannotSendWhileRecording() {
        XCTAssertTrue(AiChatMic.canSend(online: true, sending: false, draft: "こんにちは", phase: .idle))
        XCTAssertFalse(AiChatMic.canSend(online: true, sending: false, draft: "こんにちは", phase: .listening))
        XCTAssertFalse(AiChatMic.canSend(online: true, sending: false, draft: "こんにちは", phase: .transcribing))
        XCTAssertFalse(AiChatMic.canSend(online: true, sending: false, draft: "  ", phase: .idle))
        XCTAssertFalse(AiChatMic.canSend(online: true, sending: true, draft: "こんにちは", phase: .idle))
        XCTAssertFalse(AiChatMic.canSend(online: false, sending: false, draft: "こんにちは", phase: .idle))
    }

    func testSpeakerDisabledWhileRecording() {
        XCTAssertTrue(AiChatMic.canSpeak(phase: .idle))
        XCTAssertFalse(AiChatMic.canSpeak(phase: .listening))
        XCTAssertFalse(AiChatMic.canSpeak(phase: .transcribing))
    }

    /// speak: true でも録音・文字起こしの間に届いた返事は読まない(VoiceOver にも渡さない)。
    func testNoAutoReadWhileRecording() {
        XCTAssertEqual(AiChatLeaf.autoReadMode(speak: true, visible: true, voiceOver: false, recording: true), .none)
        XCTAssertEqual(AiChatLeaf.autoReadMode(speak: true, visible: true, voiceOver: true, recording: true), .none)
        XCTAssertEqual(AiChatLeaf.autoReadMode(speak: true, visible: true, voiceOver: false, recording: false), .speak)
    }

    func testDictatedTextIsAddedToDraftForReview() {
        XCTAssertEqual(DictationText.appending("こんにちは", to: ""), "こんにちは")
        XCTAssertEqual(DictationText.appending(" 元気? ", to: "やあ"), "やあ\n元気?")
    }

    func testVoiceOverLabels() {
        XCTAssertEqual(AiChatLeaf.micLabel(.start), L("話して入力"))
        XCTAssertEqual(AiChatLeaf.micLabel(.stop), L("聞いています… タップで終わる"))
        XCTAssertEqual(AiChatLeaf.micLabel(.busy), L("今は話して入力を使えません"))
        XCTAssertNotEqual(AiChatLeaf.micLabel(.prepare), AiChatLeaf.micLabel(.start))
    }
}
#endif
