import XCTest
import MetamonDSL
@testable import MetamonServices

final class MockServicesTests: XCTestCase {
    func testInMemoryAppConfigCRUD() async throws {
        let client = InMemoryAppConfigClient(seed: MockSeed.demoRecords())
        var rows = try await client.list(appId: "a", entity: "expense")
        XCTAssertEqual(rows.count, 2)

        let created = try await client.create(appId: "a", entity: "expense", data: ["amount": 100])
        rows = try await client.list(appId: "a", entity: "expense")
        XCTAssertEqual(rows.count, 3)

        let updated = try await client.update(appId: "a", entity: "expense", id: created.id, data: ["amount": 200])
        XCTAssertEqual(updated["amount"], 200)

        try await client.delete(appId: "a", entity: "expense", id: created.id)
        rows = try await client.list(appId: "a", entity: "expense")
        XCTAssertEqual(rows.count, 2)

        do {
            _ = try await client.update(appId: "a", entity: "expense", id: "nope", data: [:])
            XCTFail("expected 404")
        } catch APIError.http(let status, _) {
            XCTAssertEqual(status, 404)
        }
    }

    func testMockAIClientRouting() {
        XCTAssertEqual(MockAIClient.sampleName(for: "筋トレを記録したい"), "training")
        XCTAssertEqual(MockAIClient.sampleName(for: "散歩の写真を残したい"), "walk")
        XCTAssertEqual(MockAIClient.sampleName(for: "最初に戻して"), "initial")
        XCTAssertEqual(MockAIClient.sampleName(for: "家計簿"), "kakeibo")
    }

    @MainActor func testMockTranscriber() async throws {
        let t = MockTranscriber(cannedText: "テスト", delay: .zero)
        XCTAssertTrue(t.isReady)
        XCTAssertEqual(t.status, .ready)
        try await t.startRecording()
        let text = try await t.stopAndTranscribe()
        XCTAssertEqual(text, "テスト")
    }

    func testNotificationTimeParsing() {
        XCTAssertEqual(UserNotificationScheduler.parseTime("08:30")?.0, 8)
        XCTAssertEqual(UserNotificationScheduler.parseTime("08:30")?.1, 30)
        XCTAssertNil(UserNotificationScheduler.parseTime("25:00"))
        XCTAssertNil(UserNotificationScheduler.parseTime(nil))
        XCTAssertEqual(UserNotificationScheduler.weekdayNumber("sun"), 1)
        XCTAssertEqual(UserNotificationScheduler.weekdayNumber(nil), 2)
    }

    func testWhisperVariantResolution() {
        XCTAssertEqual(WhisperKitRecognizer.resolveVariant("small"), "openai_whisper-small")
        XCTAssertEqual(WhisperKitRecognizer.resolveVariant("openai_whisper-large-v3"), "openai_whisper-large-v3")
        XCTAssertEqual(WhisperKitRecognizer.resolveVariant("distil-whisper_distil-large-v3"), "distil-whisper_distil-large-v3")
    }
}
