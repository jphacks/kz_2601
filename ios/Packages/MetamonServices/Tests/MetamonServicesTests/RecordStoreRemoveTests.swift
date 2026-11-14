import XCTest
import MetamonDSL
@testable import MetamonServices

/// RecordStore の最終更新(lastUpdatedAt)と、アプリ単位・全部の削除(removeAll(appId:)・removeEverything)。
final class RecordStoreRemoveTests: XCTestCase {
    private var root: URL!
    private var store: RecordStore!
    private var videos: LocalVideoStore!
    private var audios: LocalAudioStore!

    override func setUp() {
        root = FileManager.default.temporaryDirectory.appendingPathComponent("metamon-rsr-\(UUID().uuidString)")
        videos = LocalVideoStore(directory: root.appendingPathComponent("videos"))
        audios = LocalAudioStore(directory: root.appendingPathComponent("audios"))
        store = RecordStore(directory: root.appendingPathComponent("records"), videos: videos, audios: audios)
    }

    override func tearDown() {
        try? FileManager.default.removeItem(at: root)
    }

    private func row(_ app: String, _ entity: String, _ id: String, updatedAt: String? = nil,
                     op: PendingOp = .none, data: [String: JSONValue] = [:]) -> LocalRecord {
        LocalRecord(appId: app, entity: entity, recordId: id, data: data, updatedAt: updatedAt, pendingOp: op)
    }

    private func touch(_ url: URL?) throws {
        let u = try XCTUnwrap(url)
        try FileManager.default.createDirectory(at: u.deletingLastPathComponent(), withIntermediateDirectories: true)
        try Data([1]).write(to: u)
    }

    // MARK: lastUpdatedAt

    func testLastUpdatedAtIsNilWhenEmpty() async {
        let v = await store.lastUpdatedAt(appId: "a", entity: "memo")
        XCTAssertNil(v)
    }

    func testLastUpdatedAtIsMaxOfSyncedRowsOnly() async {
        await store.upsert(row("a", "memo", "1", updatedAt: "2026-01-01T00:00:00Z"))
        await store.upsert(row("a", "memo", "2", updatedAt: "2026-03-01T00:00:00Z"))
        await store.upsert(row("a", "memo", "3", updatedAt: "2026-02-01T00:00:00Z"))
        // 未送信の行は基準に入れない(新しくても無視)
        await store.upsert(row("a", "memo", "4", updatedAt: "2027-01-01T00:00:00Z", op: .update))
        await store.upsert(row("a", "memo", "5", updatedAt: "2027-06-01T00:00:00Z", op: .delete))
        let v = await store.lastUpdatedAt(appId: "a", entity: "memo")
        XCTAssertEqual(v, "2026-03-01T00:00:00Z")
    }

    func testLastUpdatedAtIgnoresRowsWithoutUpdatedAt() async {
        await store.upsert(row("a", "memo", "1", updatedAt: nil))
        let none = await store.lastUpdatedAt(appId: "a", entity: "memo")
        XCTAssertNil(none)
        await store.upsert(row("a", "memo", "2", updatedAt: "2026-01-01T00:00:00Z"))
        let some = await store.lastUpdatedAt(appId: "a", entity: "memo")
        XCTAssertEqual(some, "2026-01-01T00:00:00Z")
    }

    func testLastUpdatedAtIsPerEntityAndPerApp() async {
        await store.upsert(row("a", "memo", "1", updatedAt: "2026-01-01T00:00:00Z"))
        await store.upsert(row("a", "todo", "1", updatedAt: "2026-05-01T00:00:00Z"))
        await store.upsert(row("b", "memo", "1", updatedAt: "2026-09-01T00:00:00Z"))
        let am = await store.lastUpdatedAt(appId: "a", entity: "memo")
        let at = await store.lastUpdatedAt(appId: "a", entity: "todo")
        let bm = await store.lastUpdatedAt(appId: "b", entity: "memo")
        XCTAssertEqual(am, "2026-01-01T00:00:00Z")
        XCTAssertEqual(at, "2026-05-01T00:00:00Z")
        XCTAssertEqual(bm, "2026-09-01T00:00:00Z")
    }

    // MARK: removeAll(appId:)

    func testRemoveAllDeletesOnlyThatApp() async {
        await store.upsert(row("a", "memo", "1"))
        await store.upsert(row("a", "todo", "1"))
        await store.upsert(row("b", "memo", "1"))
        await store.removeAll(appId: "a")
        let am = await store.rows(appId: "a", entity: "memo")
        let at = await store.rows(appId: "a", entity: "todo")
        let bm = await store.rows(appId: "b", entity: "memo")
        XCTAssertTrue(am.isEmpty)
        XCTAssertTrue(at.isEmpty)
        XCTAssertEqual(bm.map(\.recordId), ["1"], "他のアプリには触れない")
    }

    func testRemoveAllDeletesFilesNotYetLoaded() async {
        await store.upsert(row("a", "memo", "1"))
        await store.upsert(row("b", "memo", "1"))
        // 何も読み込んでいない新しい RecordStore から消す
        let fresh = RecordStore(directory: root.appendingPathComponent("records"), videos: videos, audios: audios)
        await fresh.removeAll(appId: "a")
        let reopened = RecordStore(directory: root.appendingPathComponent("records"), videos: videos, audios: audios)
        let a = await reopened.rows(appId: "a", entity: "memo")
        let b = await reopened.rows(appId: "b", entity: "memo")
        XCTAssertTrue(a.isEmpty, "ファイルごと消える")
        XCTAssertEqual(b.map(\.recordId), ["1"])
    }

    func testRemoveAllDeletesThatAppsLocalMediaButKeepsOthers() async throws {
        try touch(videos.url(for: "vid-a"))
        try touch(videos.url(for: "vid-b"))
        try touch(audios.url(for: "aud-a"))
        try touch(audios.url(for: "aud-b"))
        await store.upsert(row("a", "memo", "1", data: ["clip": .string("video:vid-a|data:image/jpeg;base64,AAAA"), "voice": .string("audio:aud-a|3")]))
        await store.upsert(row("b", "memo", "1", data: ["clip": .string("video:vid-b"), "voice": .string("audio:aud-b|3")]))
        await store.removeAll(appId: "a")
        XCTAssertFalse(videos.exists("vid-a"))
        XCTAssertFalse(audios.exists("aud-a"))
        XCTAssertTrue(videos.exists("vid-b"), "他のアプリの動画は残る")
        XCTAssertTrue(audios.exists("aud-b"), "他のアプリの録音は残る")
    }

    func testRemoveAllOnUnknownAppIsHarmless() async {
        await store.upsert(row("a", "memo", "1"))
        await store.removeAll(appId: "nothing")
        let kept = await store.rows(appId: "a", entity: "memo")
        XCTAssertEqual(kept.count, 1)
    }

    // MARK: removeEverything

    func testRemoveEverythingClearsAllAppsAndPending() async {
        await store.upsert(row("a", "memo", "1"))
        await store.upsert(row("b", "todo", "1", op: .create))
        await store.removeEverything()
        let a = await store.rows(appId: "a", entity: "memo")
        let b = await store.rows(appId: "b", entity: "todo")
        let pending = await store.pendingCount(appId: "b")
        XCTAssertTrue(a.isEmpty)
        XCTAssertTrue(b.isEmpty)
        XCTAssertEqual(pending, 0)
        let reopened = RecordStore(directory: root.appendingPathComponent("records"), videos: videos, audios: audios)
        let again = await reopened.rows(appId: "a", entity: "memo")
        XCTAssertTrue(again.isEmpty, "ディスクからも消える")
        XCTAssertFalse(FileManager.default.fileExists(atPath: root.appendingPathComponent("records").path))
    }

    func testRemoveEverythingDeletesAllLocalMedia() async throws {
        try touch(videos.url(for: "v1"))
        try touch(audios.url(for: "a1"))
        await store.upsert(row("a", "memo", "1", data: ["clip": .string("video:v1")]))
        await store.removeEverything()
        XCTAssertFalse(videos.exists("v1"))
        XCTAssertFalse(audios.exists("a1"))
    }

    func testStoreIsUsableAfterRemoveEverything() async {
        await store.upsert(row("a", "memo", "1"))
        await store.removeEverything()
        await store.upsert(row("a", "memo", "2"))
        let rows = await store.rows(appId: "a", entity: "memo")
        XCTAssertEqual(rows.map(\.recordId), ["2"])
    }
}
