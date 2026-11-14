import XCTest
import MetamonServices
@testable import Metamon

/// 有料機能(#77)を含む依頼: 無料の人は無料版で作り一言出す・Pro の人は全文で作り一言を出さない
@MainActor
final class MacComposeProTests: XCTestCase {
    private static let full = "みんなで書ける掲示板を作って"
    private static let free = "自分だけの掲示板を作って(無料版)"

    /// ready で有料機能の印付きの返事を返し、作る指示を覚える AI(ネットにはつながない)
    private actor RecordingAI: AIClient {
        private(set) var morphed: [String] = []
        func morph(transcript: String, appId: String, currentVersion: Int) async throws -> MorphResult {
            morphed.append(transcript)
            throw CancellationError()
        }
        func chat(appId: String, messages: [ChatMessage], locale: String?, mode: ChatMode?) async throws -> ChatTurn {
            ChatTurn(kind: .ready, text: "これで作ります", turn: 1, transcript: MacComposeProTests.full,
                     requiresPro: true, freeTranscript: MacComposeProTests.free)
        }
        func resetChat(appId: String) async {}
    }

    private func session(_ ai: any AIClient, isPro: Bool) -> MacComposeSession {
        MacComposeSession(target: .create, ai: ai, reduceMotion: { true }, isConsentGranted: { true }, isPro: { isPro },
                          clarifyEnabled: true, clarifyAutoBuild: false, historyStore: ChatHistoryStore())
    }

    private func waitUntil(_ condition: () async -> Bool) async throws {
        for _ in 0..<150 { if await condition() { return }; try await Task.sleep(for: .milliseconds(20)) }
    }

    func testInstructionUsesFreeOnlyWhenNotPro() {
        XCTAssertEqual(MacComposeSession.instruction(requiresPro: true, full: "a", free: "b", isPro: false), "b")
        XCTAssertEqual(MacComposeSession.instruction(requiresPro: true, full: "a", free: "b", isPro: true), "a")
        XCTAssertEqual(MacComposeSession.instruction(requiresPro: false, full: "a", free: "b", isPro: false), "a")
        XCTAssertEqual(MacComposeSession.instruction(requiresPro: true, full: "a", free: nil, isPro: false), "a")
        XCTAssertEqual(MacComposeSession.instruction(requiresPro: true, full: "a", free: "", isPro: false), "a")
    }

    private func confirmed(isPro: Bool) async throws -> (drops: Bool, morphed: [String]) {
        let ai = RecordingAI()
        let s = session(ai, isPro: isPro)
        s.send("掲示板を作りたい")
        try await waitUntil { s.latest != nil && !s.sending }
        let drops = s.dropsPaidFeatures
        s.confirm()
        try await waitUntil { await !ai.morphed.isEmpty }
        return (drops, await ai.morphed)
    }

    func testFreeUserBuildsFreeVersionWithNote() async throws {
        let r = try await confirmed(isPro: false)
        XCTAssertTrue(r.drops, "無料の人には「Mac ではまだ使えない」を出す")
        XCTAssertEqual(r.morphed.count, 1)
        XCTAssertTrue(r.morphed[0].contains(Self.free))
        XCTAssertFalse(r.morphed[0].contains(Self.full))
    }

    func testProUserBuildsFullVersionWithoutNote() async throws {
        let r = try await confirmed(isPro: true)
        XCTAssertFalse(r.drops, "Pro の人には出さない")
        XCTAssertEqual(r.morphed.count, 1)
        XCTAssertTrue(r.morphed[0].contains(Self.full))
        XCTAssertFalse(r.morphed[0].contains(Self.free))
    }
}
