import XCTest
import MetamonDSL
@testable import MetamonRenderer
@testable import Metamon

/// iOS と Mac で共有する保存・削除の前段(#939 `Shared/Runtime/RecordActionSteps.swift`)。
@MainActor
final class RecordActionStepsTests: XCTestCase {
    private func dsl() throws -> AppDSL {
        let json = """
        {"schemaVersion":1,"app":{"name":"メモ","voiceEntry":"floating"},"initialScreen":"form",
         "entities":[{"id":"memo","name":"メモ","fields":[{"id":"body","name":"本文","type":"text","required":true}]},
                     {"id":"profile","name":"プロフィール","single":true,"fields":[{"id":"name","name":"名前","type":"text"}]}],
         "screens":[{"id":"form","title":"メモ","entity":"memo","components":[{"type":"textField","field":"body"}]}]}
        """
        return try JSONDecoder().decode(AppDSL.self, from: Data(json.utf8))
    }

    private let route = Route(screen: "form", stack: "sheet")

    func testSaveWithoutFormIsInvalid() throws {
        let target = RecordActionSteps.saveTarget(Action(type: .saveRecord, entity: "memo"), ActionContext(route: route),
                                                  dsl: try dsl()) { _ in [] }
        guard case .invalid(let message) = target else { return XCTFail("フォームが無ければ保存しない") }
        XCTAssertEqual(message, L10n.tr("保存するフォームがありません"))
    }

    func testSaveWithMissingRequiredIsInvalidAndFlags() throws {
        let d = try dsl()
        let draft = FormDraft(entity: try XCTUnwrap(d.entity("memo")))
        let target = RecordActionSteps.saveTarget(Action(type: .saveRecord, entity: "memo"), ActionContext(route: route, form: draft),
                                                  dsl: d) { _ in [] }
        guard case .invalid = target else { return XCTFail("必須が空なら保存しない") }
        XCTAssertEqual(draft.flaggedFields, ["body"])
    }

    func testFilledNewFormIsReadyAsNew() throws {
        let d = try dsl()
        let draft = FormDraft(entity: try XCTUnwrap(d.entity("memo")))
        draft.values["body"] = .string("牛乳")
        let target = RecordActionSteps.saveTarget(Action(type: .saveRecord, entity: "memo"), ActionContext(route: route, form: draft),
                                                  dsl: d) { _ in [] }
        guard case .ready(let entity, _, let recordId) = target else { return XCTFail("保存できる") }
        XCTAssertEqual(entity, "memo")
        XCTAssertNil(recordId)
    }

    func testSingleRecordIsNeverDeleted() throws {
        let ctx = ActionContext(route: route, recordId: "r1")
        XCTAssertNil(RecordActionSteps.deleteTarget(Action(type: .deleteRecord, entity: "profile"), ctx, dsl: try dsl()))
        let memo = RecordActionSteps.deleteTarget(Action(type: .deleteRecord, entity: "memo"), ctx, dsl: try dsl())
        XCTAssertEqual(memo?.entity, "memo")
        XCTAssertEqual(memo?.recordId, "r1")
    }
}
