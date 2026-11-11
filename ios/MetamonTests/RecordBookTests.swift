import XCTest
import SwiftUI
import Observation
import MetamonDSL
@testable import MetamonRenderer
import MetamonServices
@testable import Metamon

/// Mac のランタイムの記録の読み書き(`Shared/Runtime/RecordBook.swift`・#1338)。
/// Mac にはまだ単体テストのターゲットが無い(#1212)ので、Mac と同じ型を iOS のテストで確かめる。
/// 圏外のエンジンを使う(列を流さないので、作った記録の id がサーバーの id に替わらない)。
@MainActor
final class RecordBookTests: XCTestCase {
    private let appId = "test-book-\(UUID().uuidString.lowercased())"
    private let allowAll: (String, Record) -> Bool = { _, _ in true }

    private func kakeibo() throws -> AppDSL {
        let url = try XCTUnwrap(Bundle(for: AppRuntime.self).url(forResource: "kakeibo", withExtension: "json"),
                                "docs/dsl/examples の kakeibo.json がアプリに入っていない")
        return try AppDSL.load(contentsOf: url)
    }

    private func engine() -> RecordSyncEngine {
        let dir = FileManager.default.temporaryDirectory.appendingPathComponent(appId, isDirectory: true)
        addTeardownBlock { try? FileManager.default.removeItem(at: dir) }
        let store = RecordStore(directory: dir)
        let client = InMemoryAppConfigClient()
        return RecordSyncEngine(store: store, queue: SyncQueue(directory: dir, store: store, client: client),
                                client: client, connectivity: ManualConnectivity(online: false))
    }

    private func book(_ dsl: AppDSL) -> RecordBook {
        RecordBook(appId: appId, dsl: dsl, recordSync: engine())
    }

    private func newExpense(_ dsl: AppDSL, amount: Double, memo: String) throws -> FormDraft {
        let form = FormDraft(entity: try XCTUnwrap(dsl.entity("expense")))
        form.values["amount"] = .number(amount)
        form.values["memo"] = .string(memo)
        return form
    }

    // MARK: - 追加・編集・何も変えない(M10)

    func testSaveNewShowsInListThenEditOverwrites() async throws {
        let dsl = try kakeibo()
        let b = book(dsl)
        let form = try newExpense(dsl, amount: 500, memo: "コンビニ")
        let added = await b.save(entity: "expense", form: form, recordId: nil, canEdit: allowAll)
        XCTAssertEqual(added, .saved)
        let first = try XCTUnwrap(b.records(for: "expense").first, "保存した記録が一覧に出る")
        XCTAssertEqual(b.records(for: "expense").count, 1)
        XCTAssertEqual(first.data["memo"], .string("コンビニ"))
        XCTAssertEqual(form.lastSaveResult, .queuedOffline, "圏外なので送信待ち")

        let edit = FormDraft(entity: try XCTUnwrap(dsl.entity("expense")), record: first)
        edit.values["memo"] = .string("スーパー")
        let edited = await b.save(entity: "expense", form: edit, recordId: first.id, canEdit: allowAll)
        XCTAssertEqual(edited, .saved)
        XCTAssertEqual(b.records(for: "expense").map(\.id), [first.id], "同じ記録を上書き(増えない)")
        XCTAssertEqual(b.records(for: "expense").first?.data["memo"], .string("スーパー"))
    }

    func testSaveUnchangedExistingDoesNotWrite() async throws {
        let dsl = try kakeibo()
        let b = book(dsl)
        _ = await b.save(entity: "expense", form: try newExpense(dsl, amount: 300, memo: "電車"), recordId: nil, canEdit: allowAll)
        let saved = try XCTUnwrap(b.records(for: "expense").first)
        let queued = await b.recordSync.queue.count

        let edit = FormDraft(entity: try XCTUnwrap(dsl.entity("expense")), record: saved)
        let outcome = await b.save(entity: "expense", form: edit, recordId: saved.id, canEdit: allowAll)
        XCTAssertEqual(outcome, .unchanged, "書かずに画面だけ進める")
        XCTAssertNil(edit.lastSaveResult, "「保存しました」を出さない")
        let after = await b.recordSync.queue.count
        XCTAssertEqual(after, queued, "送信の列に積まない")
        XCTAssertEqual(b.records(for: "expense").first?.updatedAt, saved.updatedAt)
    }

    func testSaveOthersRecordIsReadOnlyAndNotWritten() async throws {
        let dsl = try kakeibo()
        let b = book(dsl)
        _ = await b.save(entity: "expense", form: try newExpense(dsl, amount: 300, memo: "電車"), recordId: nil, canEdit: allowAll)
        let saved = try XCTUnwrap(b.records(for: "expense").first)
        let edit = FormDraft(entity: try XCTUnwrap(dsl.entity("expense")), record: saved)
        edit.values["memo"] = .string("書き換え")
        let outcome = await b.save(entity: "expense", form: edit, recordId: saved.id) { _, _ in false }
        XCTAssertEqual(outcome, .readOnly)
        XCTAssertEqual(b.records(for: "expense").first?.data["memo"], .string("電車"))
    }

    // MARK: - 複製して追加

    func testDuplicateAddMakesAnotherRecord() async throws {
        let dsl = try kakeibo()
        let b = book(dsl)
        _ = await b.save(entity: "expense", form: try newExpense(dsl, amount: 800, memo: "ランチ"), recordId: nil, canEdit: allowAll)
        let original = try XCTUnwrap(b.records(for: "expense").first)

        // 同じ内容を新規のフォームに写して保存(recordId は持たない)
        let copy = FormDraft(entity: try XCTUnwrap(dsl.entity("expense")))
        copy.values = original.data
        XCTAssertFalse(copy.isEditing)
        let outcome = await b.save(entity: "expense", form: copy, recordId: nil, canEdit: allowAll)
        XCTAssertEqual(outcome, .saved)
        let ids = b.records(for: "expense").map(\.id)
        XCTAssertEqual(ids.count, 2, "上書きせず 1 件増える")
        XCTAssertEqual(Set(ids).count, 2, "別の id")
        XCTAssertEqual(b.records(for: "expense").filter { $0.data["memo"] == .string("ランチ") }.count, 2)
    }

    // MARK: - 連打

    func testDoubleTapSaveMakesOneRecord() async throws {
        let dsl = try kakeibo()
        let b = book(dsl)
        let form = try newExpense(dsl, amount: 120, memo: "お茶")
        let first = Task { await b.save(entity: "expense", form: form, recordId: nil, canEdit: allowAll) }
        let second = Task { await b.save(entity: "expense", form: form, recordId: nil, canEdit: allowAll) }
        let outcomes = [await first.value, await second.value]
        XCTAssertEqual(outcomes.filter { $0 == .saved }.count, 1, "書くのは 1 回だけ: \(outcomes)")
        XCTAssertEqual(b.records(for: "expense").count, 1, "2 件できない")
        let queued = await b.recordSync.queue.count
        XCTAssertEqual(queued, 1)
    }

    // MARK: - 削除 → 取り消し(M11)

    func testDeleteThenUndoRestoresSameId() async throws {
        let dsl = try kakeibo()
        let b = book(dsl)
        _ = await b.save(entity: "expense", form: try newExpense(dsl, amount: 1000, memo: "本"), recordId: nil, canEdit: allowAll)
        let id = try XCTUnwrap(b.records(for: "expense").first?.id)

        let ticket = await b.deleteUndoably(entity: "expense", recordId: id, grace: .seconds(60))
        XCTAssertEqual(ticket.recordId, id)
        XCTAssertTrue(b.records(for: "expense").isEmpty, "一覧からすぐ消える")

        let undone = await b.undoDelete(ticket)
        XCTAssertTrue(undone)
        XCTAssertEqual(b.records(for: "expense").map(\.id), [id], "同じ id で戻る")
    }

    func testUndoAfterCommitDoesNothing() async throws {
        let dsl = try kakeibo()
        let b = book(dsl)
        _ = await b.save(entity: "expense", form: try newExpense(dsl, amount: 1000, memo: "本"), recordId: nil, canEdit: allowAll)
        let id = try XCTUnwrap(b.records(for: "expense").first?.id)
        let ticket = await b.deleteUndoably(entity: "expense", recordId: id, grace: .seconds(60))
        await b.recordSync.commitPendingDeletions() // 時間切れ
        let undone = await b.undoDelete(ticket)
        XCTAssertFalse(undone, "送った後は戻らない")
        XCTAssertTrue(b.records(for: "expense").isEmpty)
    }

    // MARK: - update / create

    func testUpdateMissingAndReadOnlyDoNotWrite() async throws {
        let dsl = try kakeibo()
        let b = book(dsl)
        let missing = await b.update(entity: "expense", recordId: "nope", data: ["memo": .string("x")], canEdit: allowAll)
        XCTAssertEqual(missing, .missing)
        _ = await b.save(entity: "expense", form: try newExpense(dsl, amount: 1, memo: "a"), recordId: nil, canEdit: allowAll)
        let id = try XCTUnwrap(b.records(for: "expense").first?.id)
        let readOnly = await b.update(entity: "expense", recordId: id, data: ["memo": .string("b")]) { _, _ in false }
        XCTAssertEqual(readOnly, .readOnly)
        XCTAssertEqual(b.records(for: "expense").first?.data["memo"], .string("a"))
        let saved = await b.update(entity: "expense", recordId: id, data: ["memo": .string("c")], canEdit: allowAll)
        XCTAssertEqual(saved, .saved)
        XCTAssertEqual(b.records(for: "expense").first?.data["memo"], .string("c"))
    }

    func testCreateOnlyForEntityInDSL() async throws {
        let b = book(try kakeibo())
        let unknown = await b.create(entity: "nope", data: ["x": .number(1)])
        XCTAssertFalse(unknown)
        XCTAssertTrue(b.records(for: "nope").isEmpty)
        let created = await b.create(entity: "expense", data: ["amount": .number(1)])
        XCTAssertTrue(created)
        XCTAssertEqual(b.records(for: "expense").count, 1)
    }
}

// MARK: - 楽観的な保存の失敗(OptimisticSave)

/// `MacAppRuntime.updateRecord` と同じつなぎ方で `RecordBook` を使うランタイム。ホストへの一言を数える。
@MainActor
@Observable
private final class BookRuntime: DSLRuntime {
    let dsl: AppDSL
    let book: RecordBook
    var paths: [String: [Route]] = [:]
    var selectedTab = ""
    var loading: Set<String> { book.loading }
    var uid: String?
    var saveFailed = 0
    var readOnly = 0

    init(dsl: AppDSL, book: RecordBook) {
        self.dsl = dsl
        self.book = book
    }

    var currentUserId: String? { uid }
    func records(for entity: String) -> [Record] { book.records(for: entity) }
    func load(entities: Set<String>, force: Bool) async { await book.load(entities: entities, force: force) }
    func run(_ action: Action, _ ctx: ActionContext) async {}
    func close(_ route: Route) {}
    func pathBinding(_ stack: String) -> Binding<[Route]> { .constant([]) }

    // MacAppRuntime.updateRecord と同じ
    func updateRecord(entity: String, recordId: String, data: [String: JSONValue]) async -> Bool {
        switch await book.update(entity: entity, recordId: recordId, data: data, canEdit: { self.canEdit(entity: $0, record: $1) }) {
        case .saved: return true
        case .missing: return false
        case .readOnly:
            notifyReadOnly(entity: entity)
            return false
        }
    }

    func notifyReadOnly(entity: String) { readOnly += 1 }
    func notifySaveFailed(entity: String) { saveFailed += 1 }
}

@MainActor
final class RecordBookOptimisticSaveTests: XCTestCase {
    private let appId = "test-book-opt-\(UUID().uuidString.lowercased())"

    /// 共有の買い物リスト(作った人だけが変更できる)
    private func runtime() async throws -> BookRuntime {
        let json = """
        {"schemaVersion":1,"app":{"name":"買い物","voiceEntry":"floating"},"initialScreen":"home",
         "entities":[{"id":"item","name":"品物","scope":"shared","fields":[{"id":"name","name":"名前","type":"string"},
                                                                       {"id":"done","name":"済み","type":"boolean"}]}],
         "screens":[{"id":"home","title":"買い物","components":[{"type":"list","entity":"item"}]}]}
        """
        let dsl = try JSONDecoder().decode(AppDSL.self, from: Data(json.utf8))
        let dir = FileManager.default.temporaryDirectory.appendingPathComponent(appId, isDirectory: true)
        addTeardownBlock { try? FileManager.default.removeItem(at: dir) }
        let store = RecordStore(directory: dir)
        let client = InMemoryAppConfigClient()
        let sync = RecordSyncEngine(store: store, queue: SyncQueue(directory: dir, store: store, client: client),
                                    client: client, connectivity: ManualConnectivity(online: false))
        for (id, author) in [("mine1", "me"), ("mine2", "me"), ("theirs", "other")] {
            await store.upsert(LocalRecord(appId: appId, entity: "item", recordId: id, data: ["done": .bool(false)],
                                           createdAt: "1", author: author))
        }
        let rt = BookRuntime(dsl: dsl, book: RecordBook(appId: appId, dsl: dsl, recordSync: sync))
        rt.uid = "me"
        await rt.book.reloadLocal("item")
        XCTAssertEqual(rt.records(for: "item").count, 3)
        return rt
    }

    private func done(_ rt: BookRuntime, _ id: String) -> JSONValue? {
        rt.records(for: "item").first { $0.id == id }?.data["done"]
    }

    func testSavedUpdateDoesNotNotify() async throws {
        let rt = try await runtime()
        let saved = await OptimisticSave.update(rt, entity: "item", recordId: "mine1", data: ["done": .bool(true)])
        XCTAssertTrue(saved)
        XCTAssertEqual(done(rt, "mine1"), .bool(true))
        XCTAssertEqual(rt.saveFailed, 0)
        XCTAssertEqual(rt.readOnly, 0)
    }

    func testMissingRecordNotifiesSaveFailedOnce() async throws {
        let rt = try await runtime()
        let saved = await OptimisticSave.update(rt, entity: "item", recordId: "gone", data: ["done": .bool(true)])
        XCTAssertFalse(saved, "呼び出し側が表示を戻す")
        XCTAssertEqual(rt.saveFailed, 1)
        XCTAssertEqual(rt.readOnly, 0)
    }

    func testOthersRecordNotifiesReadOnlyOnly() async throws {
        let rt = try await runtime()
        let saved = await OptimisticSave.update(rt, entity: "item", recordId: "theirs", data: ["done": .bool(true)])
        XCTAssertFalse(saved)
        XCTAssertEqual(rt.readOnly, 1, "作った人だけが変更できます")
        XCTAssertEqual(rt.saveFailed, 0, "保存できませんでしたを重ねない")
        XCTAssertEqual(done(rt, "theirs"), .bool(false), "書かない")
    }

    func testInOrderStopsAtFailureAndNotifiesOnce() async throws {
        let rt = try await runtime()
        var settled: [String] = []
        let ok = await OptimisticSave.updateInOrder(rt, entity: "item", writes: [
            ("mine1", ["done": .bool(true)]), ("gone", ["done": .bool(true)]), ("mine2", ["done": .bool(true)]),
        ]) { settled.append($0) }
        XCTAssertFalse(ok)
        XCTAssertEqual(settled, ["mine1", "gone", "mine2"], "全件を落ち着かせる")
        XCTAssertEqual(rt.saveFailed, 1, "一言は 1 回だけ")
        XCTAssertEqual(done(rt, "mine1"), .bool(true))
        XCTAssertEqual(done(rt, "mine2"), .bool(false), "失敗の後は送らない")
    }
}
