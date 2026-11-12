import XCTest
import MetamonDSL
import MetamonServices
@testable import Metamon

/// Mac の App Intents(#1034): iOS と同じ Intent の型が Mac の `IntentHost` を通って、一覧の選択・会話・記録の書き込みにつながる。
@MainActor
final class MacIntentsTests: XCTestCase {
    private var original: IntentHost!
    private var writer: FakeRecordWriter!
    private var library: MacAppLibrary!

    override func setUp() async throws {
        original = IntentHost.shared
        writer = FakeRecordWriter()
        library = MacAppLibrary(store: nil)
        library.apply(AppDocument(appId: "mine", version: 2, dsl: try kakeibo()), created: true)
        let host = IntentHost(records: writer, server: { nil })
        host.register(library: library, bundled: [SampleApp(id: "example", name: "読書メモ", description: nil, url: nil)])
        IntentHost.shared = host
    }

    override func tearDown() async throws {
        IntentHost.shared = original
    }

    private func kakeibo() throws -> AppDSL {
        let url = try XCTUnwrap(Bundle(for: MacAppRuntime.self).url(forResource: "kakeibo", withExtension: "json"))
        return try AppDSL.load(contentsOf: url)
    }

    // MARK: - 開く / 話しかける

    func testOpenIntentQueuesOpenForWindow() async throws {
        let intent = OpenTailorAppIntent()
        intent.target = TailorAppEntity(AppSummary(id: "mine", name: "家計簿", currentVersion: 2))
        _ = try await intent.perform()
        XCTAssertEqual(IntentHost.shared.take(), .open(appId: "mine"))
        XCTAssertNil(IntentHost.shared.take(), "1 つのウインドウだけが受け取る")
    }

    func testTalkIntentQueuesTalk() async throws {
        _ = try await TalkToTailorItIntent().perform()
        XCTAssertEqual(IntentHost.shared.pending, .talk)
    }

    func testRouteOpenSelectsKnownAppOnly() {
        var selected: [String] = []
        var talked = 0
        MacIntentRoute.apply(.open(appId: "mine"), appIds: ["mine", "example"], select: { selected.append($0) }, talk: { talked += 1 })
        MacIntentRoute.apply(.open(appId: "gone"), appIds: ["mine", "example"], select: { selected.append($0) }, talk: { talked += 1 })
        XCTAssertEqual(selected, ["mine"])
        XCTAssertEqual(talked, 0)
        MacIntentRoute.apply(.talk, appIds: [], select: { selected.append($0) }, talk: { talked += 1 })
        XCTAssertEqual(talked, 1)
        XCTAssertEqual(selected, ["mine"])
    }

    // MARK: - 候補

    func testAppQueryListsCreatedThenExamples() async throws {
        let apps = try await TailorAppQuery().suggestedEntities()
        XCTAssertEqual(apps.map(\.id), ["mine", "example"])
        let name = try XCTUnwrap(library.generated["mine"]?.dsl.app.name)
        let hit = try await TailorAppQuery().entities(matching: name)
        XCTAssertEqual(hit.map(\.id), ["mine"])
    }

    func testRecordTargetsAreCreatedAppsOnly() async throws {
        let targets = try await RecordTargetQuery().suggestedEntities()
        XCTAssertFalse(targets.isEmpty)
        XCTAssertTrue(targets.allSatisfy { $0.appId == "mine" }, "同梱の例(メモリ上の記録)には足さない")
    }

    // MARK: - 記録を追加

    func testAddRecordIntentWritesThroughWriter() async throws {
        let targets = try await RecordTargetQuery().suggestedEntities()
        let target = try XCTUnwrap(targets.first)
        let intent = AddRecordIntent()
        intent.target = target
        intent.content = "ランチ 800円"
        let added = expectation(forNotification: IntentHost.recordAdded, object: "mine")
        _ = try await intent.perform()
        await fulfillment(of: [added], timeout: 1)

        XCTAssertEqual(writer.added.count, 1)
        let call = try XCTUnwrap(writer.added.first)
        XCTAssertEqual(call.appId, "mine")
        XCTAssertEqual(call.entity, target.entityId)
        XCTAssertTrue(call.data.values.contains(.number(800)), "文中の数値を数値の項目に入れる: \(call.data)")
        XCTAssertTrue(call.encryptedFields.isEmpty, "端末だけのアプリは暗号化しない")
    }

    func testAddRecordToExampleIsRejected() async throws {
        let host = IntentHost.shared
        do {
            _ = try await host.addRecord(appId: "example", entity: "x", data: [:], encryptedFields: [:])
            XCTFail("同梱の例には足さない")
        } catch {}
        XCTAssertTrue(writer.added.isEmpty)
    }

    func testNotReadyBeforeRegister() async throws {
        IntentHost.shared = IntentHost(records: writer, server: { nil })
        XCTAssertFalse(IntentHost.shared.canWriteRecords)
        let suggested = try await TailorAppQuery().suggestedEntities()
        XCTAssertTrue(suggested.isEmpty)
        do {
            _ = try await IntentHost.shared.addRecord(appId: "mine", entity: "x", data: [:], encryptedFields: [:])
            XCTFail("一覧を渡す前は書けない")
        } catch let error as TailorIntentError {
            guard case .notReady = error else { return XCTFail("\(error)") }
        }
    }
}

@MainActor
private final class FakeRecordWriter: MacIntentRecordWriting {
    struct Call {
        var appId: String
        var entity: String
        var data: [String: JSONValue]
        var encryptedFields: [String: Set<String>]
    }

    var added: [Call] = []

    func cached(appId: String, entity: String, server: (any AppConfigClient)?) async -> [Record] { [] }

    func add(appId: String, entity: String, data: [String: JSONValue],
             encryptedFields: [String: Set<String>], server: (any AppConfigClient)?) async -> Bool {
        added.append(Call(appId: appId, entity: entity, data: data, encryptedFields: encryptedFields))
        return true
    }
}
