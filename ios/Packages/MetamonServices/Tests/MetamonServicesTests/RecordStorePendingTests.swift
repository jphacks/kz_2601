import XCTest
import MetamonDSL
@testable import MetamonServices

/// 端末の記録の削除中の印と戻し(`RecordStore.markDeleting`・`revertPending`)、仮の id(`LocalRecord.temporaryId`・`isKnown`)。
final class RecordStorePendingTests: XCTestCase {
    private var dir: URL!
    private var store: RecordStore!
    private let app = "app-1", entity = "memo"

    override func setUp() async throws {
        dir = FileManager.default.temporaryDirectory.appendingPathComponent("metamon-pending-\(UUID().uuidString)")
        store = RecordStore(directory: dir)
    }

    override func tearDown() async throws {
        try? FileManager.default.removeItem(at: dir)
    }

    private func row(_ id: String, _ op: PendingOp = .none, clientId: String? = nil) -> LocalRecord {
        LocalRecord(appId: app, entity: entity, recordId: id, data: ["title": .string(id)],
                    createdAt: "2026-11-01T00:00:00Z", pendingOp: op, clientId: clientId)
    }

    private func op(_ id: String) async -> PendingOp? { await store.row(appId: app, entity: entity, recordId: id)?.pendingOp }
    private func visible() async -> [String] { await store.records(appId: app, entity: entity).map(\.id) }

    // MARK: - temporaryId・isKnown

    func testTemporaryIdAndIsTemporary() {
        XCTAssertEqual(LocalRecord.temporaryId(clientId: "abc"), "local-abc")
        XCTAssertTrue(row(LocalRecord.temporaryId(clientId: "abc"), .create, clientId: "abc").isTemporary)
        XCTAssertFalse(row("srv-1").isTemporary)
    }

    func testIsKnownByServerIdAndTemporaryId() {
        let synced = row("srv-1", clientId: "abc")
        let cases: [(id: String, want: Bool)] = [
            ("srv-1", true),
            ("local-abc", true),
            ("abc", false),
            ("local-xyz", false),
            ("", false),
        ]
        for c in cases { XCTAssertEqual(synced.isKnown(as: c.id), c.want, c.id) }
        XCTAssertTrue(row("srv-2").isKnown(as: "srv-2"))
        XCTAssertFalse(row("srv-2").isKnown(as: "local-"), "clientId の無い行は今の id だけ")
    }

    func testCurrentFindsRowByTemporaryIdAfterSync() async {
        let temp = LocalRecord.temporaryId(clientId: "abc")
        await store.upsert(row(temp, .create, clientId: "abc"))
        await store.markSynced(appId: app, entity: entity, localId: temp, server: Record(id: "srv-1", data: [:]))
        let byTemp = await store.current(appId: app, entity: entity, recordId: temp)
        XCTAssertEqual(byTemp?.recordId, "srv-1")
        XCTAssertEqual(byTemp?.clientId, "abc")
        let byRow = await store.row(appId: app, entity: entity, recordId: temp)
        XCTAssertNil(byRow, "row は今の id だけで引く")
    }

    // MARK: - markDeleting

    func testMarkDeletingHidesRowAndCountsAsPending() async {
        await store.upsert(row("srv-1"))
        await store.upsert(row("srv-2"))
        await store.markDeleting(appId: app, entity: entity, recordId: "srv-1")
        let marked = await op("srv-1")
        XCTAssertEqual(marked, .delete)
        let shown = await visible()
        XCTAssertEqual(shown, ["srv-2"])
        let count = await store.pendingCount(appId: app)
        XCTAssertEqual(count, 1)
    }

    func testMarkDeletingByTemporaryIdAfterSync() async {
        let temp = LocalRecord.temporaryId(clientId: "abc")
        await store.upsert(row(temp, .create, clientId: "abc"))
        await store.markSynced(appId: app, entity: entity, localId: temp, server: Record(id: "srv-1", data: [:]))
        // 画面が仮 id を持ったまま消した
        await store.markDeleting(appId: app, entity: entity, recordId: temp)
        let marked = await op("srv-1")
        XCTAssertEqual(marked, .delete)
        let shown = await visible()
        XCTAssertEqual(shown, [])
    }

    func testMarkDeletingUnknownIdChangesNothing() async {
        await store.upsert(row("srv-1"))
        await store.markDeleting(appId: app, entity: entity, recordId: "nope")
        let kept = await op("srv-1")
        XCTAssertEqual(kept, PendingOp.none)
        let count = await store.pendingCount(appId: app)
        XCTAssertEqual(count, 0)
    }

    func testMarkDeletingIsSavedToDisk() async {
        await store.upsert(row("srv-1"))
        await store.markDeleting(appId: app, entity: entity, recordId: "srv-1")
        let reopened = RecordStore(directory: dir)
        let r = await reopened.row(appId: app, entity: entity, recordId: "srv-1")
        XCTAssertEqual(r?.pendingOp, .delete)
    }

    // MARK: - revertPending

    func testRevertPendingTable() async {
        let cases: [(op: PendingOp, exists: Bool, after: PendingOp?)] = [
            (.create, false, nil),
            (.update, true, PendingOp.none),
            (.delete, true, PendingOp.none),
            (.none, true, PendingOp.none),
        ]
        for c in cases {
            let id = "r-\(c.op.rawValue)"
            await store.upsert(row(id, c.op))
            await store.revertPending(appId: app, entity: entity, recordId: id)
            let after = await op(id)
            XCTAssertEqual(after, c.after, "\(c.op)")
            let shown = await visible()
            XCTAssertEqual(shown.contains(id), c.exists, "\(c.op)")
        }
        let count = await store.pendingCount(appId: app)
        XCTAssertEqual(count, 0)
    }

    func testRevertPendingAfterMarkDeletingBringsRowBack() async {
        await store.upsert(row("srv-1"))
        await store.markDeleting(appId: app, entity: entity, recordId: "srv-1")
        await store.revertPending(appId: app, entity: entity, recordId: "srv-1")
        let shown = await visible()
        XCTAssertEqual(shown, ["srv-1"])
        let r = await store.row(appId: app, entity: entity, recordId: "srv-1")
        XCTAssertEqual(r?.data["title"], .string("srv-1"), "中身はそのまま")
    }

    func testRevertPendingUnknownIdChangesNothing() async {
        await store.upsert(row("srv-1", .update))
        await store.revertPending(appId: app, entity: entity, recordId: "nope")
        let kept = await op("srv-1")
        XCTAssertEqual(kept, .update)
    }
}
