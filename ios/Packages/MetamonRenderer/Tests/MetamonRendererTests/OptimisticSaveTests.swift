import XCTest
import SwiftUI
import Observation
import MetamonDSL
@testable import MetamonRenderer

/// 保存できるかを id ごとに決め、ホストへの通知を数えるランタイム。
@MainActor
@Observable
private final class SaveFailRuntime: DSLRuntime {
    var dsl: AppDSL
    var paths: [String: [Route]] = [:]
    var selectedTab = ""
    var loading: Set<String> = []
    var store: [String: [Record]]
    var failing: Set<String> = []
    var uid: String?
    var updates: [String] = []
    var saveFailed = 0
    var readOnly = 0

    init(dsl: AppDSL, store: [String: [Record]]) {
        self.dsl = dsl
        self.store = store
    }

    var currentUserId: String? { uid }
    func records(for entity: String) -> [Record] { store[entity] ?? [] }
    func load(entities: Set<String>, force: Bool) async {}
    func run(_ action: Action, _ ctx: ActionContext) async {}
    func close(_ route: Route) {}
    func pathBinding(_ stack: String) -> Binding<[Route]> { .constant([]) }
    func updateRecord(entity: String, recordId: String, data: [String: JSONValue]) async -> Bool {
        updates.append(recordId)
        guard let r = store[entity]?.first(where: { $0.id == recordId }) else { return false }
        guard canEdit(entity: entity, record: r) else { notifyReadOnly(entity: entity); return false }
        return !failing.contains(recordId)
    }
    func notifyReadOnly(entity: String) { readOnly += 1 }
    func notifySaveFailed(entity: String) { saveFailed += 1 }
}

/// 楽観的に描いた変更が保存できなかった時の一言(一覧のチェック・単語カード・トーナメント)。1 操作 1 回・read-only とは重ねない。
final class OptimisticSaveTests: XCTestCase {
    private func tournament() throws -> AppDSL {
        var url = URL(fileURLWithPath: #filePath)
        for _ in 0..<6 { url.deleteLastPathComponent() }
        return try AppDSL.load(contentsOf: url.appending(path: "docs/dsl/examples/tournament.json"))
    }

    private let entrants = [
        Record(id: "a", data: ["wins": .number(1)], author: "me"),
        Record(id: "b", data: ["wins": .number(0)], author: "me"),
    ]

    @MainActor private func runtime() throws -> SaveFailRuntime {
        SaveFailRuntime(dsl: try tournament(), store: ["entrant": entrants])
    }

    @MainActor func testSavedWriteDoesNotNotify() async throws {
        let rt = try runtime()
        let saved = await OptimisticSave.update(rt, entity: "entrant", recordId: "a", data: ["wins": .number(2)])
        XCTAssertTrue(saved)
        XCTAssertEqual(rt.saveFailed, 0)
    }

    /// 一覧のチェック・単語カードと同じ 1 件の更新: 失敗したら 1 回だけ知らせる。
    @MainActor func testFailedWriteNotifiesOnce() async throws {
        let rt = try runtime()
        rt.failing = ["a"]
        let saved = await OptimisticSave.update(rt, entity: "entrant", recordId: "a", data: ["wins": .number(2)])
        XCTAssertFalse(saved)
        XCTAssertEqual(rt.saveFailed, 1)
        XCTAssertEqual(rt.readOnly, 0)
    }

    /// 他人の共有記録は「作った人だけ」がホストから出るので、保存失敗の一言は重ねない。
    @MainActor func testReadOnlyFailureDoesNotNotifySaveFailed() async throws {
        let rt = try runtime()
        rt.dsl.entities[0].scope = "shared"
        rt.uid = "someone-else"
        let saved = await OptimisticSave.update(rt, entity: "entrant", recordId: "a", data: ["wins": .number(2)])
        XCTAssertFalse(saved)
        XCTAssertEqual(rt.readOnly, 1)
        XCTAssertEqual(rt.saveFailed, 0)
    }

    /// トーナメントの勝者の入れ替えは 2 件書く: 1 件目で失敗したら 2 件目は送らず、一言は 1 回。保存待ちは両方外す。
    @MainActor func testBracketFirstWriteFailureNotifiesOnce() async throws {
        let rt = try runtime()
        rt.failing = ["b"]
        var settled: [String] = []
        let saved = await BracketLeaf.save(["a": 2, "b": 1], winsField: "wins", entity: "entrant", rt: rt) { settled.append($0) }
        XCTAssertFalse(saved)
        XCTAssertEqual(rt.updates, ["b"], "下げる方が先・失敗したら残りは送らない")
        XCTAssertEqual(settled, ["b", "a"])
        XCTAssertEqual(rt.saveFailed, 1)
    }

    /// 2 件目で失敗しても一言は 1 回。
    @MainActor func testBracketSecondWriteFailureNotifiesOnce() async throws {
        let rt = try runtime()
        rt.failing = ["a"]
        let saved = await BracketLeaf.save(["a": 2, "b": 1], winsField: "wins", entity: "entrant", rt: rt) { _ in }
        XCTAssertFalse(saved)
        XCTAssertEqual(rt.updates, ["b", "a"])
        XCTAssertEqual(rt.saveFailed, 1)
    }

    @MainActor func testBracketAllSavedDoesNotNotify() async throws {
        let rt = try runtime()
        let saved = await BracketLeaf.save(["a": 2, "b": 1], winsField: "wins", entity: "entrant", rt: rt) { _ in }
        XCTAssertTrue(saved)
        XCTAssertEqual(rt.saveFailed, 0)
    }
}
