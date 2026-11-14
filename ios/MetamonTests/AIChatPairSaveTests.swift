#if !APP_STORE_REVIEW
import XCTest
import MetamonDSL
import MetamonServices
@testable import MetamonRenderer
@testable import Metamon

/// roleField の無い aiChat(stg の古い DSL)は役を並び順で読む。1 往復を対で保存し、片方が断られたらもう片方も消して交互を崩さない。
@MainActor
final class AIChatPairSaveTests: XCTestCase {
    private let appId = "test-aichat-\(UUID().uuidString.lowercased())"

    private func runtime() throws -> AppRuntime {
        let json = """
        {"schemaVersion":1,"app":{"name":"相談"},"initialScreen":"home",
         "entities":[{"id":"turn","name":"発言","fields":[{"id":"text","name":"本文","type":"text","required":true}]}],
         "screens":[{"id":"home","title":"相談","components":[]}]}
        """
        let dsl = try JSONDecoder().decode(AppDSL.self, from: Data(json.utf8))
        return AppRuntime(document: AppDocument(appId: appId, version: 1, dsl: dsl), services: BuildConfig.make(environment: .mock))
    }

    private func roles(_ rt: AppRuntime) async -> [AIChat.Role] {
        let rows = await rt.recordSync.cached(appId: appId, entity: "turn")
        return AIChat.turns(rows, textField: "text", roleField: nil).map(\.role)
    }

    func testPairSavesBothInOrder() async throws {
        let rt = try runtime()
        let ok = await rt.saveAIChatPair(entity: "turn", user: ["text": "眠れない"], assistant: ["text": "いつからですか"])
        XCTAssertTrue(ok)
        let rows = await rt.recordSync.cached(appId: appId, entity: "turn")
        XCTAssertEqual(AIChat.turns(rows, textField: "text", roleField: nil).map(\.text), ["眠れない", "いつからですか"])
        XCTAssertEqual(rt.aiChatPartners.count, 2, "どちらが断られても相方を引ける")
    }

    func testUnknownEntityLeavesNothing() async throws {
        let rt = try runtime()
        let ok = await rt.saveAIChatPair(entity: "nope", user: ["text": "a"], assistant: ["text": "b"])
        XCTAssertFalse(ok)
        XCTAssertTrue(rt.aiChatPartners.isEmpty)
    }

    /// 返事の保存がサーバーに断られた(キューがローカル行を消した)ら、自分の発言も消す。以後の役は入れ替わらない。
    func testRefusedReplyRemovesUserTurnAndKeepsAlternation() async throws {
        let rt = try runtime()
        _ = await rt.saveAIChatPair(entity: "turn", user: ["text": "1"], assistant: ["text": "2"])
        let saved = await rt.saveAIChatPairIds(entity: "turn", user: ["text": "3"], assistant: ["text": "4"])
        let second = try XCTUnwrap(saved)
        let aiId = second.assistant
        _ = await rt.recordSync.delete(appId: appId, entity: "turn", recordId: aiId)
        await rt.revertAIChatPartner(for: .planLimit(appId: appId, entity: "turn", limit: PlanLimit(resource: .recordsPerApp), recordId: aiId))
        let rows = await rt.recordSync.cached(appId: appId, entity: "turn")
        XCTAssertEqual(AIChat.turns(rows, textField: "text", roleField: nil).map(\.text), ["1", "2"], "自分の発言(3)だけを残さない")
        _ = await rt.saveAIChatPair(entity: "turn", user: ["text": "5"], assistant: ["text": "6"])
        let roles = await roles(rt)
        XCTAssertEqual(roles, [.user, .assistant, .user, .assistant], "断られた後も自分 → AI の交互のまま")
        XCTAssertEqual(rt.aiChatPartners.count, 4, "消した対は忘れる")
    }

    func testRefusalOfUnpairedRecordIsIgnored() async throws {
        let rt = try runtime()
        _ = await rt.saveAIChatPair(entity: "turn", user: ["text": "1"], assistant: ["text": "2"])
        await rt.revertAIChatPartner(for: .rejected(appId: appId, entity: "turn", message: "bad", recordId: "local-other"))
        await rt.revertAIChatPartner(for: .rejected(appId: appId, entity: "turn", message: "bad"))
        let rows = await rt.recordSync.cached(appId: appId, entity: "turn")
        XCTAssertEqual(rows.count, 2)
    }

}
#endif
