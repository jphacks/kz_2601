import XCTest
import MetamonDSL
import MetamonRenderer
import MetamonServices
@testable import Metamon

/// Field.health の記録を Mac で保存した時の一言(#958): アプリごとに 1 回・health のある項目だけ・VoiceOver でも読む
@MainActor
final class MacHealthNoteTests: XCTestCase {
    private var posted: [String] = []
    private var savedPost: ((String, MacVoiceOver.Priority) -> Void)!
    private var suiteName = ""
    private var defaults: UserDefaults!

    override func setUp() async throws {
        savedPost = MacVoiceOver.post
        posted = []
        MacVoiceOver.post = { [weak self] text, _ in self?.posted.append(text) }
        suiteName = "mac-health-note-\(UUID().uuidString)"
        defaults = UserDefaults(suiteName: suiteName)
    }

    override func tearDown() async throws {
        MacVoiceOver.post = savedPost
        defaults.removePersistentDomain(forName: suiteName)
    }

    private let json = """
    {"schemaVersion":1,"app":{"name":"体重"},"initialScreen":"form",
     "entities":[
       {"id":"weights","name":"体重","fields":[{"id":"kg","name":"体重","type":"number","health":"weight"},
                                               {"id":"memo","name":"メモ","type":"string"}]},
       {"id":"notes","name":"メモ","fields":[{"id":"memo","name":"メモ","type":"string"}]}],
     "screens":[{"id":"form","title":"体重","entity":"weights","components":[{"type":"numberField","field":"kg"}]}]}
    """

    private func runtime(appId: String = "test-health-\(UUID().uuidString.lowercased())") throws -> MacAppRuntime {
        let dsl = try JSONDecoder().decode(AppDSL.self, from: Data(json.utf8))
        let rt = MacAppRuntime(appId: appId, dsl: dsl, ai: MockAIClient(delay: .zero), isAIConsentGranted: { false })
        rt.healthNote = MacHealthNote(defaults: defaults)
        return rt
    }

    private func save(_ rt: MacAppRuntime, entity: String, _ values: [String: JSONValue]) async throws {
        let form = FormDraft(entity: try XCTUnwrap(rt.dsl.entity(entity)))
        for (k, v) in values { form.values[k] = v }
        let action = try JSONDecoder().decode(Action.self, from: Data(#"{"type":"saveRecord","entity":"\#(entity)"}"#.utf8))
        await rt.run(action, ActionContext(route: Route(screen: "form"), form: form))
    }

    func testShowsOncePerApp() async throws {
        let rt = try runtime()
        try await save(rt, entity: "weights", ["kg": .number(60)])
        XCTAssertEqual(rt.records(for: "weights").count, 1, "記録は保存する")
        XCTAssertEqual(rt.toast, MacHealthNote.message)
        XCTAssertEqual(posted, [MacAnnouncement.saved.text, MacHealthNote.message], "保存しました に続けて読む")
        XCTAssertEqual(rt.announcedToast, MacHealthNote.message, "toast の側では読み直さない")

        rt.toast = nil
        posted = []
        try await save(rt, entity: "weights", ["kg": .number(61)])
        XCTAssertEqual(rt.records(for: "weights").count, 2)
        XCTAssertEqual(rt.toast, MacAnnouncement.saved.text, "2 回目は出さない(根なので「保存しました」だけ・#1884)")
        XCTAssertEqual(posted, [MacAnnouncement.saved.text])

        let other = try runtime()
        try await save(other, entity: "weights", ["kg": .number(60)])
        XCTAssertEqual(other.toast, MacHealthNote.message, "覚えるのはアプリごと")
    }

    func testRemembersAcrossRuntimes() async throws {
        let first = try runtime(appId: "test-health-same")
        try await save(first, entity: "weights", ["kg": .number(60)])
        XCTAssertEqual(first.toast, MacHealthNote.message)
        let reopened = try runtime(appId: "test-health-same")
        try await save(reopened, entity: "weights", ["kg": .number(60)])
        XCTAssertEqual(reopened.toast, MacAnnouncement.saved.text, "開き直しても同じアプリには出さない(UserDefaults)")
    }

    func testOnlyForHealthFields() async throws {
        let rt = try runtime()
        try await save(rt, entity: "notes", ["memo": .string("買い物")])
        XCTAssertEqual(rt.records(for: "notes").count, 1)
        XCTAssertEqual(rt.toast, MacAnnouncement.saved.text, "health の無い entity では出さない")
        try await save(rt, entity: "weights", ["memo": .string("朝")])
        XCTAssertEqual(rt.toast, MacAnnouncement.saved.text, "health の項目が空なら書く物が無いので出さない")
        XCTAssertEqual(posted, [MacAnnouncement.saved.text, MacAnnouncement.saved.text])
        XCTAssertFalse(defaults.bool(forKey: MacHealthNote.key(rt.appId)), "出していないので印も付けない")
    }

    func testAppliesOnlyToNewRecordsWithHealthValue() throws {
        let dsl = try JSONDecoder().decode(AppDSL.self, from: Data(json.utf8))
        let weights = dsl.entity("weights")
        XCTAssertTrue(MacHealthNote.applies(entity: weights, data: ["kg": .number(60)], isNew: true))
        XCTAssertFalse(MacHealthNote.applies(entity: weights, data: ["kg": .number(60)], isNew: false), "iOS も編集では書かない")
        XCTAssertFalse(MacHealthNote.applies(entity: weights, data: ["kg": .null], isNew: true))
        XCTAssertFalse(MacHealthNote.applies(entity: dsl.entity("notes"), data: ["memo": .string("a")], isNew: true))
        XCTAssertFalse(MacHealthNote.applies(entity: nil, data: [:], isNew: true))
    }
}
