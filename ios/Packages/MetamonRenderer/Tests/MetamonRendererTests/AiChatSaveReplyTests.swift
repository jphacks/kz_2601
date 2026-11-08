#if !APP_STORE_REVIEW
import XCTest
import SwiftUI
import Observation
import MetamonDSL
@testable import MetamonRenderer

/// 保存の結果を返すランタイム(`createRecord` が失敗する時・対で保存するホスト)。
@MainActor
@Observable
private final class SavingRuntime: DSLRuntime {
    var dsl: AppDSL
    var paths: [String: [Route]] = [:]
    var selectedTab = ""
    var loading: Set<String> = []
    var created: [[String: JSONValue]] = []
    var fails = false

    init(dsl: AppDSL) { self.dsl = dsl }

    func records(for entity: String) -> [Record] { [] }
    func load(entities: Set<String>, force: Bool) async {}
    func run(_ action: Action, _ ctx: ActionContext) async {}
    func close(_ route: Route) {}
    func pathBinding(_ stack: String) -> Binding<[Route]> { .constant([]) }
    func createRecord(entity: String, data: [String: JSONValue]) async -> Bool {
        guard !fails else { return false }
        created.append(data)
        return true
    }
}

@MainActor
@Observable
private final class PairRuntime: DSLRuntime, AIChatPairSaving {
    var dsl: AppDSL
    var paths: [String: [Route]] = [:]
    var selectedTab = ""
    var loading: Set<String> = []
    var created: [[String: JSONValue]] = []
    var pairs: [([String: JSONValue], [String: JSONValue])] = []
    var pairOK = true

    init(dsl: AppDSL) { self.dsl = dsl }

    func records(for entity: String) -> [Record] { [] }
    func load(entities: Set<String>, force: Bool) async {}
    func run(_ action: Action, _ ctx: ActionContext) async {}
    func close(_ route: Route) {}
    func pathBinding(_ stack: String) -> Binding<[Route]> { .constant([]) }
    func createRecord(entity: String, data: [String: JSONValue]) async -> Bool {
        created.append(data)
        return true
    }

    func saveAIChatPair(entity: String, user: [String: JSONValue], assistant: [String: JSONValue]) async -> Bool {
        pairs.append((user, assistant))
        return pairOK
    }
}

/// aiChat の返事の保存(`AiChatLeaf.saveReply`): 保存の結果を捨てない・roleField が無い時は対で保存する。
@MainActor
final class AiChatSaveReplyTests: XCTestCase {
    private func dsl() throws -> AppDSL {
        let json = """
        {"schemaVersion":1,"app":{"name":"相談"},"initialScreen":"home",
         "entities":[{"id":"turn","name":"発言","fields":[{"id":"text","name":"本文","type":"text","required":true}]}],
         "screens":[{"id":"home","title":"相談","components":[]}]}
        """
        return try JSONDecoder().decode(AppDSL.self, from: Data(json.utf8))
    }

    func testRoleFieldSavesOnlyReplyAndReportsFailure() async throws {
        let rt = SavingRuntime(dsl: try dsl())
        let ok = await AiChatLeaf.saveReply("どうしましたか", after: "眠れない", rt: rt, entity: "turn", textField: "text",
                                            roleField: "role", base: [:])
        XCTAssertTrue(ok)
        XCTAssertEqual(rt.created, [["text": "どうしましたか", "role": "assistant"]], "自分の発言は送る前に保存済み")

        rt.fails = true
        let failed = await AiChatLeaf.saveReply("どうしましたか", after: "眠れない", rt: rt, entity: "turn", textField: "text",
                                                roleField: "role", base: [:])
        XCTAssertFalse(failed, "保存できなかった返事を成功扱いにしない")
    }

    func testEmptyReplyIsNotSaved() async throws {
        let rt = PairRuntime(dsl: try dsl())
        let ok = await AiChatLeaf.saveReply("  ", after: "眠れない", rt: rt, entity: "turn", textField: "text", roleField: nil, base: [:])
        XCTAssertFalse(ok)
        XCTAssertTrue(rt.pairs.isEmpty, "返事が空なら自分の発言だけを残さない(交互がずれる)")
        XCTAssertTrue(rt.created.isEmpty)
    }

    /// roleField が無い(古い DSL): 役は並び順で読むので、ホストに対で保存させる。
    func testWithoutRoleFieldSavesAsPair() async throws {
        let rt = PairRuntime(dsl: try dsl())
        let ok = await AiChatLeaf.saveReply("どうしましたか", after: "眠れない", rt: rt, entity: "turn", textField: "text",
                                            roleField: nil, base: ["mood": "ふつう"])
        XCTAssertTrue(ok)
        XCTAssertEqual(rt.pairs.count, 1)
        XCTAssertEqual(rt.pairs.first?.0, ["text": "眠れない", "mood": "ふつう"])
        XCTAssertEqual(rt.pairs.first?.1, ["text": "どうしましたか", "mood": "ふつう"])
        XCTAssertTrue(rt.created.isEmpty, "1 件ずつ createRecord しない")

        rt.pairOK = false
        let failed = await AiChatLeaf.saveReply("どうしましたか", after: "眠れない", rt: rt, entity: "turn", textField: "text",
                                                roleField: nil, base: [:])
        XCTAssertFalse(failed)
    }

    /// 対の保存を持たないランタイムは 2 件続けて保存し、1 件目に失敗したら 2 件目を作らない。
    func testFallbackWithoutPairHost() async throws {
        let rt = SavingRuntime(dsl: try dsl())
        let ok = await AiChatLeaf.saveReply("どうしましたか", after: "眠れない", rt: rt, entity: "turn", textField: "text",
                                            roleField: nil, base: [:])
        XCTAssertTrue(ok)
        XCTAssertEqual(rt.created, [["text": "眠れない"], ["text": "どうしましたか"]])
        rt.created = []
        rt.fails = true
        let failed = await AiChatLeaf.saveReply("どうしましたか", after: "眠れない", rt: rt, entity: "turn", textField: "text",
                                                roleField: nil, base: [:])
        XCTAssertFalse(failed)
        XCTAssertTrue(rt.created.isEmpty)
    }
}
#endif
