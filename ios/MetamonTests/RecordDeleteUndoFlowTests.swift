import XCTest
import MetamonDSL
import MetamonRenderer
import MetamonServices
@testable import Metamon

/// 一覧の長押し「削除」→ トーストの「元に戻す」(M11)。
@MainActor
final class RecordDeleteUndoFlowTests: XCTestCase {
    private let appId = "test-undo-\(UUID().uuidString.lowercased())"

    private func runtime() throws -> AppRuntime {
        let json = """
        {"schemaVersion":1,"app":{"name":"メモ","voiceEntry":"floating"},"initialScreen":"home",
         "entities":[{"id":"memo","name":"メモ","fields":[{"id":"title","name":"題","type":"string"}]}],
         "screens":[{"id":"home","title":"メモ","components":[{"type":"list","entity":"memo","swipeToDelete":true}]}]}
        """
        let dsl = try JSONDecoder().decode(AppDSL.self, from: Data(json.utf8))
        return AppRuntime(document: AppDocument(appId: appId, version: 1, dsl: dsl), services: BuildConfig.make(environment: .mock))
    }

    func testDeleteFromListThenUndoRestoresWithoutSending() async throws {
        let rt = try runtime()
        await rt.recordSync.store.upsert(LocalRecord(appId: appId, entity: "memo", recordId: "m1", data: ["title": "牛乳"], createdAt: "1"))

        let ctx = ActionContext(route: Route(screen: "home", stack: "main"), recordId: "m1")
        await rt.run(Action(type: .deleteRecord, entity: "memo"), ctx)
        XCTAssertEqual(rt.records["memo"]?.count, 0, "一覧からすぐ消える")
        let undo = try XCTUnwrap(rt.recordUndo, "削除しました・元に戻す が出る")
        XCTAssertEqual(undo.ticket.recordId, "m1")
        let queued = await rt.recordSync.queue.count
        XCTAssertEqual(queued, 0, "戻せる間は送信の列に積まない")

        await rt.undoRecordDelete()
        XCTAssertNil(rt.recordUndo)
        XCTAssertEqual(rt.records["memo"]?.map(\.id), ["m1"], "同じ id で戻る")
        await rt.recordSync.commitPendingDeletions()
        let after = await rt.recordSync.queue.count
        XCTAssertEqual(after, 0, "戻した削除は送らない")
    }

    func testUndoAfterCommitDoesNothing() async throws {
        let rt = try runtime()
        await rt.recordSync.store.upsert(LocalRecord(appId: appId, entity: "memo", recordId: "m2", data: [:], createdAt: "1"))
        await rt.run(Action(type: .deleteRecord, entity: "memo"), ActionContext(route: Route(screen: "home", stack: "main"), recordId: "m2"))
        await rt.recordSync.commitPendingDeletions() // 背面に回った・時間切れ
        await rt.undoRecordDelete()
        let shown = await rt.recordSync.cached(appId: appId, entity: "memo")
        XCTAssertTrue(shown.isEmpty, "送った後は戻らない")
    }
}
