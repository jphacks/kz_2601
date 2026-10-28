import XCTest
import MetamonDSL
@testable import MetamonServices

/// #420: レコード削除・録り直し・アプリ削除で端末内の録音ファイルも消す(動画 #206 と同じ)。
@MainActor
final class LocalAudioCleanupTests: XCTestCase {
    private var dir: URL!
    private var engine: RecordSyncEngine!
    private var audios: LocalAudioStore { engine.store.audios }

    override func setUp() async throws {
        dir = FileManager.default.temporaryDirectory.appendingPathComponent("audio-cleanup-\(UUID().uuidString)")
        engine = RecordSyncEngine(client: InMemoryAppConfigClient(), connectivity: ManualConnectivity(online: false), directory: dir)
    }

    override func tearDown() async throws {
        try? FileManager.default.removeItem(at: dir)
    }

    private func makeAudio() throws -> String {
        let src = LocalAudioStore.temporaryRecordingURL()
        try Data([1, 2, 3]).write(to: src)
        return try audios.importFile(at: src)
    }

    private func value(_ id: String) -> JSONValue { .string(AudioValue(id: id, seconds: 12).encoded) }

    func testStoreIsUnderTheEngineDirectory() {
        XCTAssertTrue(audios.directory.path.hasPrefix(dir.path), "テストの録音は一時ディレクトリに置く(実機の録音を消さない)")
    }

    func testDeleteRecordRemovesAudioFile() async throws {
        let id = try makeAudio()
        let rows = await engine.save(appId: "a", entity: "memo", recordId: nil, data: ["voice": value(id)])
        XCTAssertTrue(audios.exists(id))
        _ = await engine.delete(appId: "a", entity: "memo", recordId: rows[0].id)
        XCTAssertFalse(audios.exists(id))
    }

    func testRerecordRemovesOldFileOnly() async throws {
        let old = try makeAudio(), new = try makeAudio()
        let rows = await engine.save(appId: "a", entity: "memo", recordId: nil, data: ["voice": value(old)])
        _ = await engine.save(appId: "a", entity: "memo", recordId: rows[0].id, data: ["voice": value(new)])
        XCTAssertFalse(audios.exists(old))
        XCTAssertTrue(audios.exists(new))
    }

    func testPurgeAppRemovesAudioFiles() async throws {
        let id = try makeAudio()
        _ = await engine.save(appId: "a", entity: "memo", recordId: nil, data: ["voice": value(id)])
        await engine.purge(appId: "a")
        XCTAssertFalse(audios.exists(id))
    }
}
