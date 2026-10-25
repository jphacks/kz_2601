import XCTest
import MetamonDSL
@testable import MetamonServices

/// #206: レコード削除・撮り直し・アプリ削除・アカウント削除で端末内の動画ファイルも消す。
@MainActor
final class LocalVideoCleanupTests: XCTestCase {
    private var dir: URL!
    private var engine: RecordSyncEngine!
    private var videos: LocalVideoStore { engine.store.videos }

    override func setUp() async throws {
        dir = FileManager.default.temporaryDirectory.appendingPathComponent("video-cleanup-\(UUID().uuidString)")
        engine = RecordSyncEngine(client: InMemoryAppConfigClient(), connectivity: ManualConnectivity(online: false), directory: dir)
    }

    override func tearDown() async throws {
        try? FileManager.default.removeItem(at: dir)
    }

    private func makeVideo() throws -> String {
        let src = FileManager.default.temporaryDirectory.appendingPathComponent(UUID().uuidString + ".mp4")
        try Data([1, 2, 3]).write(to: src)
        return try videos.importFile(at: src)
    }

    private func value(_ id: String) -> JSONValue { .string("video:\(id)|data:image/jpeg;base64,xx") }

    func testDeleteRecordRemovesVideoFile() async throws {
        let id = try makeVideo()
        let rows = await engine.save(appId: "a", entity: "clip", recordId: nil, data: ["video": value(id)])
        XCTAssertTrue(videos.exists(id))
        _ = await engine.delete(appId: "a", entity: "clip", recordId: rows[0].id)
        XCTAssertFalse(videos.exists(id))
    }

    func testReshootRemovesOldFileOnly() async throws {
        let old = try makeVideo(), new = try makeVideo()
        let rows = await engine.save(appId: "a", entity: "clip", recordId: nil, data: ["video": value(old)])
        _ = await engine.save(appId: "a", entity: "clip", recordId: rows[0].id, data: ["video": value(new)])
        XCTAssertFalse(videos.exists(old))
        XCTAssertTrue(videos.exists(new))
    }

    func testPurgeAppAndWipeRemoveVideos() async throws {
        let gone = try makeVideo(), kept = try makeVideo()
        _ = await engine.save(appId: "gone", entity: "clip", recordId: nil, data: ["video": value(gone)])
        _ = await engine.save(appId: "kept", entity: "clip", recordId: nil, data: ["video": value(kept)])
        await engine.purge(appId: "gone")
        XCTAssertFalse(videos.exists(gone))
        XCTAssertTrue(videos.exists(kept), "他のアプリの動画は残す")
        await engine.wipe()
        XCTAssertFalse(videos.exists(kept))
    }
}
