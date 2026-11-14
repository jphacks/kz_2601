import XCTest
import MetamonDSL
import MetamonServices
@testable import Metamon

/// アプリを消した時の記録の削除(`MacRecordVault.purge`)。開いているエンジン(ほかのウインドウ・送信の途中)が
/// 消した後にフォルダを作り直さないこと
@MainActor
final class MacRecordVaultPurgeTests: XCTestCase {
    private var root: URL!

    override func setUp() async throws {
        root = FileManager.default.temporaryDirectory
            .appendingPathComponent("metamon-vault-test-\(UUID().uuidString)", isDirectory: true)
    }

    override func tearDown() async throws {
        try? FileManager.default.removeItem(at: root)
    }

    private func exists(_ appId: String) -> Bool {
        FileManager.default.fileExists(atPath: MacRecordVault.directory(appId: appId, root: root).path)
    }

    /// 送っている途中で消しても、後から書かれてフォルダが戻らない
    func testPurgeWhileSendingDoesNotRecreateFolder() async throws {
        let engine = MacRecordVault.recordSync(appId: "a", root: root)
        engine.start()
        for i in 0..<20 {
            _ = await engine.save(appId: "a", entity: "memo", recordId: nil, data: ["n": .number(Double(i))])
        }
        XCTAssertTrue(exists("a"))

        await MacRecordVault.purge(appId: "a", root: root)
        XCTAssertFalse(exists("a"))
        let left = await engine.queue.count
        XCTAssertEqual(left, 0, "送信待ちは捨てる")

        try await Task.sleep(for: .milliseconds(300)) // 残った送信・状態の更新が走り終わる頃
        XCTAssertFalse(exists("a"), "消した後に送信の列やサーバーの行が書かれていない")
        withExtendedLifetime(engine) {}
    }

    /// 消すのはそのアプリの記録だけ。ほかのアプリのエンジンは動いたまま
    func testPurgeKeepsOtherApps() async throws {
        let a = MacRecordVault.recordSync(appId: "a", root: root)
        let b = MacRecordVault.recordSync(appId: "b", root: root)
        _ = await a.save(appId: "a", entity: "memo", recordId: nil, data: ["n": .number(1)])
        _ = await b.save(appId: "b", entity: "memo", recordId: nil, data: ["n": .number(2)])

        await MacRecordVault.purge(appId: "a", root: root)
        XCTAssertFalse(exists("a"))
        XCTAssertTrue(exists("b"))
        let rows = await b.cached(appId: "b", entity: "memo")
        XCTAssertEqual(rows.count, 1)
        withExtendedLifetime(a) {}
    }
}
