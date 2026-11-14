import XCTest
import MetamonDSL
import MetamonServices
@testable import Metamon

/// 保存期間を過ぎた記録を消す(`MacAppRuntime.purgeExpiredRecords(uid:now:)`・#482 / #979)。
/// 確認済みの entity だけ・期限の前は残す・確認していない entity は消さない
@MainActor
final class MacRetentionTests: XCTestCase {
    private let appId = "mac-retention-\(UUID().uuidString.lowercased())"
    private let now = Date(timeIntervalSince1970: 1_800_000_000)

    private static let json = """
    {"schemaVersion":1,"app":{"name":"日記"},"initialScreen":"home",
     "entities":[{"id":"diary","name":"日記","retentionDays":7,"fields":[{"id":"body","name":"本文","type":"text"}]},
                 {"id":"memo","name":"メモ","retentionDays":7,"fields":[{"id":"body","name":"本文","type":"text"}]},
                 {"id":"chat","name":"会話","scope":"shared","retentionDays":7,"fields":[{"id":"text","name":"本文","type":"string"}]},
                 {"id":"tags","name":"タグ","fields":[{"id":"name","name":"名前","type":"string"}]}],
     "screens":[{"id":"home","title":"日記","components":[{"type":"list","entity":"diary"}]}]}
    """

    override func tearDown() async throws {
        UserDefaults.standard.removeObject(forKey: "retention.consent.\(appId)")
    }

    private func runtime() throws -> MacAppRuntime {
        let dsl = try JSONDecoder().decode(AppDSL.self, from: Data(Self.json.utf8))
        let dir = FileManager.default.temporaryDirectory.appendingPathComponent("mac-retention-\(UUID().uuidString)")
        addTeardownBlock { try? FileManager.default.removeItem(at: dir) }
        let server = InMemoryAppConfigClient()
        let store = RecordStore(directory: dir)
        let engine = RecordSyncEngine(store: store, queue: SyncQueue(directory: nil, store: store, client: server),
                                      client: server, connectivity: ManualConnectivity(online: false))
        return MacAppRuntime(appId: appId, dsl: dsl, recordSync: engine,
                             ai: MockAIClient(delay: .zero), isAIConsentGranted: { false })
    }

    private func seed(_ rt: MacAppRuntime, entity: String, id: String, daysAgo: Double, author: String? = nil) async {
        let created = ISO8601DateFormatter().string(from: now.addingTimeInterval(-daysAgo * 86_400))
        await rt.recordSync.store.upsert(LocalRecord(appId: appId, entity: entity, recordId: id, data: [:],
                                                     createdAt: created, updatedAt: created, author: author))
    }

    private func ids(_ rt: MacAppRuntime, _ entity: String) async -> Set<String> {
        Set(await rt.recordSync.cached(appId: appId, entity: entity).map(\.id))
    }

    func testDeletesOnlyExpiredOfConfirmedEntity() async throws {
        let rt = try runtime()
        RetentionConsent.set(7, appId: appId, entity: "diary")
        await seed(rt, entity: "diary", id: "old", daysAgo: 10)
        await seed(rt, entity: "diary", id: "new", daysAgo: 1)
        await seed(rt, entity: "tags", id: "tag", daysAgo: 100) // 期間の無い entity は消さない

        await rt.purgeExpiredRecords(uid: "u-me", now: now)
        let diary = await ids(rt, "diary")
        XCTAssertEqual(diary, ["new"], "期限の前は残す")
        let tags = await ids(rt, "tags")
        XCTAssertEqual(tags, ["tag"])
    }

    func testKeepsEverythingUntilConfirmed() async throws {
        let rt = try runtime()
        await seed(rt, entity: "diary", id: "old", daysAgo: 10)
        await rt.purgeExpiredRecords(uid: "u-me", now: now)
        let unconfirmed = await ids(rt, "diary")
        XCTAssertEqual(unconfirmed, ["old"], "確認していない entity は消さない")

        // 30 日で確認済み → 7 日に短くした設定は、確認し直すまで消さない
        RetentionConsent.set(30, appId: appId, entity: "diary")
        await rt.purgeExpiredRecords(uid: "u-me", now: now)
        let shorter = await ids(rt, "diary")
        XCTAssertEqual(shorter, ["old"])
    }

    func testConfirmingOneEntityDoesNotDeleteAnother() async throws {
        let rt = try runtime()
        RetentionConsent.set(7, appId: appId, entity: "diary")
        await seed(rt, entity: "diary", id: "d-old", daysAgo: 10)
        await seed(rt, entity: "memo", id: "m-old", daysAgo: 10)

        await rt.purgeExpiredRecords(uid: "u-me", now: now)
        let diary = await ids(rt, "diary")
        XCTAssertEqual(diary, [])
        let memo = await ids(rt, "memo")
        XCTAssertEqual(memo, ["m-old"], "memo は確認していないので残す")
    }

    func testSharedEntityDeletesOnlyOwnRecords() async throws {
        let rt = try runtime()
        RetentionConsent.set(7, appId: appId, entity: "chat")
        await seed(rt, entity: "chat", id: "mine", daysAgo: 10, author: "u-me")
        await seed(rt, entity: "chat", id: "theirs", daysAgo: 10, author: "u-other")

        await rt.purgeExpiredRecords(uid: "u-me", now: now)
        let chat = await ids(rt, "chat")
        XCTAssertEqual(chat, ["theirs"], "共有は自分が作った記録だけ消す")
    }
}
