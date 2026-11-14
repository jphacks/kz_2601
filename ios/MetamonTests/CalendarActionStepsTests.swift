import XCTest
import MetamonDSL
import MetamonRenderer
import MetamonServices
@testable import Metamon

/// iOS と Mac で共有する addToCalendar / lookupBook の手順(#978)。EventKit・本の API は呼ばずフェイクを差す。
@MainActor
final class CalendarActionStepsTests: XCTestCase {
    private func dsl() throws -> AppDSL {
        let json = """
        {"schemaVersion":1,"app":{"name":"予定"},"initialScreen":"form",
         "entities":[{"id":"events","name":"予定","fields":[{"id":"title","name":"件名","type":"string"},
           {"id":"at","name":"日時","type":"datetime"},{"id":"place","name":"場所","type":"string"}]}],
         "screens":[{"id":"form","title":"予定","entity":"events","components":[{"type":"textField","field":"title"}]}]}
        """
        return try JSONDecoder().decode(AppDSL.self, from: Data(json.utf8))
    }

    private func action() throws -> Action {
        try JSONDecoder().decode(Action.self, from: Data(
            #"{"type":"addToCalendar","entity":"events","calendarEvent":{"startField":"at","locationField":"place","alertMinutes":15}}"#.utf8))
    }

    func testDraftFromFormUsesTypedValues() throws {
        let dsl = try dsl()
        let form = FormDraft(entity: try XCTUnwrap(dsl.entities.first))
        form.values = ["title": .string("歯医者"), "at": .string("2026-11-20T10:00:00Z"), "place": .string("駅前")]
        let result = CalendarActionSteps.draft(try action(), ActionContext(route: Route(screen: "form"), form: form), dsl: dsl) { _ in [] }
        guard case .ready(let draft) = result else { return XCTFail("予定にできるはず: \(result)") }
        XCTAssertEqual(draft.title, "歯医者")
        XCTAssertEqual(draft.location, "駅前")
        XCTAssertEqual(draft.end.timeIntervalSince(draft.start), 3600, "終わりが無ければ 60 分")
        XCTAssertEqual(draft.alertOffset, -900)
    }

    func testDraftFromListRow() throws {
        let dsl = try dsl()
        let rows = [Record(id: "r1", data: ["title": .string("会議"), "at": .string("2026-11-21T09:00:00Z")])]
        let result = CalendarActionSteps.draft(try action(), ActionContext(route: Route(screen: "list"), recordId: "r1"), dsl: dsl) { _ in rows }
        guard case .ready(let draft) = result else { return XCTFail("行から予定にできるはず") }
        XCTAssertEqual(draft.title, "会議")
    }

    func testMissingStartAsksToFillIn() throws {
        let dsl = try dsl()
        let form = FormDraft(entity: try XCTUnwrap(dsl.entities.first))
        form.values = ["title": .string("歯医者")]
        let result = CalendarActionSteps.draft(try action(), ActionContext(route: Route(screen: "form"), form: form), dsl: dsl) { _ in [] }
        guard case .invalid = result else { return XCTFail("始まりが空なら一言") }
    }

    func testNoRecordIsInvalid() throws {
        let result = CalendarActionSteps.draft(try action(), ActionContext(route: Route(screen: "list"), recordId: "none"), dsl: try dsl()) { _ in [] }
        guard case .invalid = result else { return XCTFail("記録が無ければ一言") }
    }

    // MARK: - 書き込み(許可 → 追加)

    private actor FakeWriter: CalendarWriting {
        let granted: Bool
        let fails: Bool
        private(set) var added: [CalendarEventDraft] = []
        init(granted: Bool, fails: Bool = false) { self.granted = granted; self.fails = fails }
        func requestWriteAccess() async throws -> Bool { granted }
        func add(_ draft: CalendarEventDraft) async throws {
            if fails { throw URLError(.cannotCreateFile) }
            added.append(draft)
        }
    }

    private func sample() throws -> CalendarEventDraft {
        let entity = try XCTUnwrap(try dsl().entities.first)
        return try CalendarEventPlan.build(CalendarEventSpec(startField: "at"), entity: entity,
                                           data: ["title": .string("歯医者"), "at": .string("2026-11-20T10:00:00Z")]).get()
    }

    func testSaveAddsWhenAllowed() async throws {
        let writer = FakeWriter(granted: true)
        let result = await CalendarActionSteps.save(try sample(), using: writer)
        XCTAssertEqual(result, .added)
        let added = await writer.added
        XCTAssertEqual(added.map(\.title), ["歯医者"])
    }

    func testSaveDoesNotWriteWhenDenied() async throws {
        let writer = FakeWriter(granted: false)
        let result = await CalendarActionSteps.save(try sample(), using: writer)
        XCTAssertEqual(result, .denied)
        let added = await writer.added
        XCTAssertTrue(added.isEmpty)
    }

    func testSaveReportsFailure() async throws {
        let result = await CalendarActionSteps.save(try sample(), using: FakeWriter(granted: true, fails: true))
        XCTAssertEqual(result, .failed)
    }

    // MARK: - 本の情報(つなぎ先が無い時)

    func testBookLookupWithoutClientFallsBackToManualEntry() async throws {
        let json = """
        {"schemaVersion":1,"app":{"name":"本棚"},"initialScreen":"form",
         "entities":[{"id":"books","name":"本","fields":[{"id":"isbn","name":"ISBN","type":"string"},{"id":"title","name":"書名","type":"string"}]}],
         "screens":[{"id":"form","title":"本","entity":"books","components":[{"type":"textField","field":"isbn"}]}]}
        """
        let dsl = try JSONDecoder().decode(AppDSL.self, from: Data(json.utf8))
        let form = FormDraft(entity: try XCTUnwrap(dsl.entities.first))
        form.values["isbn"] = .string("9784003101018")
        let action = try JSONDecoder().decode(Action.self, from: Data(#"{"type":"lookupBook","bookLookup":{"isbnField":"isbn","titleField":"title"}}"#.utf8))
        let message = await BookLookupSteps.run(action, form: form, client: nil)
        XCTAssertEqual(message, L10n.tr("本の情報を読み込めません。手で入力してください"))
        XCTAssertEqual((form.values["title"] ?? .null).displayString, "", "何も入れない")
    }
}
