import XCTest
import MetamonDSL
import MetamonServices
@testable import Metamon

/// チャット基本化(#186)の状態遷移を AppRuntime + モック AI で確かめる。
/// アプリ(DSL)が変わるのは「これでいく」を押した時だけ、を中心に見る。
@MainActor
final class ChatFirstFlowTests: XCTestCase {
    private func makeEditRuntime() throws -> AppRuntime {
        let services = BuildConfig.make(environment: .mock)
        let doc = try DSLExamples.document("kakeibo", version: 1)
        let rt = AppRuntime(document: doc, services: services)
        rt.clarifyEnabled = true
        return rt
    }

    func testEditAnswerDoesNotChangeApp() async throws {
        let rt = try makeEditRuntime()
        XCTAssertEqual(rt.chatMode, .edit)
        await rt.submitUtterance("これ現状は WebView でやってるんですか?")
        XCTAssertEqual(rt.chatTurn?.kind, .answer)
        XCTAssertEqual(rt.chatBubbles.map(\.role), [.user, .assistant])
        XCTAssertEqual(rt.document.version, 1, "答えだけではアプリは変わらない")
        XCTAssertNil(rt.morphJob)
        XCTAssertEqual(ClarifyFlow.shortcut(for: rt.chatTurn, mode: rt.chatMode), .none)
    }

    func testEditProposalChangesAppOnlyAfterChoosing() async throws {
        let rt = try makeEditRuntime()
        rt.showVoiceSheet = true
        await rt.submitUtterance("見やすくして")
        guard case .proposals(let options) = ClarifyFlow.offer(for: try XCTUnwrap(rt.chatTurn)) else {
            return XCTFail("proposal のカードにならない: \(String(describing: rt.chatTurn))")
        }
        XCTAssertEqual(options.count, 2)
        XCTAssertEqual(rt.document.version, 1, "案を出しただけではアプリは変わらない")
        XCTAssertTrue(rt.showVoiceSheet)

        await rt.choose(options[0])
        XCTAssertEqual(rt.document.version, 2, "「これでいく」で改変される")
        XCTAssertFalse(rt.isChatting, "生成を始めたら会話は閉じる")
        XCTAssertFalse(rt.showVoiceSheet, "edit は音声シートを閉じて本体側の生成中表示へ")
        XCTAssertEqual(rt.summaryBanner?.text, "「\(options[0].instruction)」を反映しました(モック)")
    }

    func testEditReadyByVoiceDoesNotAutoBuild() async throws {
        let rt = try makeEditRuntime()
        rt.clarifyAutoBuild = true
        await rt.submitUtterance("金額を大きくして", source: .voice)
        XCTAssertEqual(rt.chatTurn?.kind, .ready)
        XCTAssertFalse(rt.chatAutoBuildPending, "edit は声で進めても自動で実行しない")
        try await Task.sleep(for: ClarifyFlow.autoBuildDelay + .milliseconds(500))
        XCTAssertEqual(rt.document.version, 1)
        XCTAssertTrue(rt.isChatting)

        await rt.buildFromChat()
        XCTAssertEqual(rt.document.version, 2, "「これでいく」で反映")
    }

    func testAnsweredQuestionIsNotPartOfApplyNowInstruction() async throws {
        let rt = try makeEditRuntime()
        await rt.submitUtterance("もうちょっと大きくすることも可能?")
        XCTAssertEqual(rt.chatTurn?.kind, .answer)
        await rt.submitUtterance("金額を大きくして")
        XCTAssertEqual(rt.chatTurn?.kind, .ready)
        XCTAssertEqual(rt.chatTurn?.transcript, "金額を大きくして")
    }

    func testClarifyOffAppliesEditImmediately() async throws {
        let rt = try makeEditRuntime()
        rt.clarifyEnabled = false
        defer { rt.clarifyEnabled = true }
        XCTAssertFalse(rt.routesToChat("見やすくして"))
        await rt.morph(transcript: "金額を大きくして")
        XCTAssertFalse(rt.isChatting)
        XCTAssertEqual(rt.document.version, 2, "OFF は従来どおり一発で反映")
    }

    func testUndoStillBypassesChat() throws {
        let rt = try makeEditRuntime()
        XCTAssertFalse(rt.routesToChat("元に戻して"), "「元に戻して」等の操作はチャットを通さない")
    }
}
