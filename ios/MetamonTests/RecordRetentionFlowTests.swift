import XCTest
import MetamonDSL
import MetamonServices
@testable import Metamon

/// 記録の保存期間(#482): 確認するまで消さない → 「自動で消す」で期限切れだけ消す → 確認済みなら聞き直さない。
@MainActor
final class RecordRetentionFlowTests: XCTestCase {
    private let appId = "test-retention-\(UUID().uuidString.lowercased())"

    override func tearDown() {
        UserDefaults.standard.removeObject(forKey: "retention.consent.\(appId)")
    }

    private func runtime(days: Int = 7) throws -> AppRuntime {
        let json = """
        {"schemaVersion":1,"app":{"name":"日記","voiceEntry":"floating"},"initialScreen":"home",
         "entities":[{"id":"diary","name":"日記","retentionDays":\(days),"fields":[{"id":"body","name":"本文","type":"text"}]},
                     {"id":"tags","name":"タグ","fields":[{"id":"name","name":"名前","type":"string"}]}],
         "screens":[{"id":"home","title":"日記","components":[{"type":"list","entity":"diary"}]}]}
        """
        let dsl = try JSONDecoder().decode(AppDSL.self, from: Data(json.utf8))
        return AppRuntime(document: AppDocument(appId: appId, version: 1, dsl: dsl), services: BuildConfig.make(environment: .mock))
    }

    private func seed(_ rt: AppRuntime, entity: String, id: String, daysAgo: Double) async {
        let created = ISO8601DateFormatter().string(from: Date().addingTimeInterval(-daysAgo * 86_400))
        await rt.recordSync.store.upsert(LocalRecord(appId: appId, entity: entity, recordId: id, data: [:], createdAt: created, updatedAt: created))
    }

    private func ids(_ rt: AppRuntime, _ entity: String) async -> Set<String> {
        Set(await rt.recordSync.cached(appId: appId, entity: entity).map(\.id))
    }

    func testNothingIsDeletedUntilConfirmed() async throws {
        let rt = try runtime()
        await seed(rt, entity: "diary", id: "old", daysAgo: 10)
        await seed(rt, entity: "diary", id: "new", daysAgo: 1)
        await seed(rt, entity: "tags", id: "tag", daysAgo: 100) // 期間の無い entity は消さない

        rt.noteRetention()
        let prompt = try XCTUnwrap(rt.retentionPrompt)
        XCTAssertEqual(prompt.items.map(\.entity), ["diary"])
        XCTAssertTrue(prompt.message(english: false).contains("「日記」は1週間たった記録を自動で消します"), prompt.message(english: false))
        await rt.purgeAllExpired()
        let before = await ids(rt, "diary")
        XCTAssertEqual(before, ["old", "new"], "確認前は消さない")

        await rt.confirmRetention()
        XCTAssertNil(rt.retentionPrompt)
        let after = await ids(rt, "diary")
        XCTAssertEqual(after, ["new"])
        let tags = await ids(rt, "tags")
        XCTAssertEqual(tags, ["tag"])
        XCTAssertEqual(RetentionConsent.days(appId: appId), ["diary": 7])

        rt.noteRetention()
        XCTAssertNil(rt.retentionPrompt, "確認済みの期間は聞き直さない")
    }

    func testDeferDoesNotDeleteAndShorterPeriodAsksAgain() async throws {
        let rt = try runtime()
        await seed(rt, entity: "diary", id: "old", daysAgo: 10)
        rt.noteRetention()
        rt.deferRetention()
        rt.noteRetention()
        XCTAssertNil(rt.retentionPrompt, "あとで にした期間はこのセッションでは聞き直さない")
        await rt.purgeAllExpired()
        let kept = await ids(rt, "diary")
        XCTAssertEqual(kept, ["old"])

        // 30 日で確認済み → 7 日に短くした版は確認し直す(確認するまで 30 日のままにもしない = 消さない)
        RetentionConsent.set(30, appId: appId, entity: "diary")
        let shorter = try runtime(days: 7)
        shorter.noteRetention()
        XCTAssertNotNil(shorter.retentionPrompt)
    }
}
