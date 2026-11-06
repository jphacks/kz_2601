import XCTest
import MetamonDSL
@testable import MetamonRenderer
@testable import Metamon

/// M10: 必須が空のまま保存すると、トーストに加えて欄に印が付き、記録は作らない。
@MainActor
final class FormRequiredSaveTests: XCTestCase {
    private let appId = "test-required-\(UUID().uuidString.lowercased())"

    private func runtime() throws -> AppRuntime {
        let json = """
        {"schemaVersion":1,"app":{"name":"メモ","voiceEntry":"floating"},"initialScreen":"form",
         "entities":[{"id":"memo","name":"メモ","fields":[{"id":"body","name":"本文","type":"text","required":true}]}],
         "screens":[{"id":"form","title":"メモ","entity":"memo","components":[{"type":"textField","field":"body"}]}]}
        """
        let dsl = try JSONDecoder().decode(AppDSL.self, from: Data(json.utf8))
        return AppRuntime(document: AppDocument(appId: appId, version: 1, dsl: dsl), services: BuildConfig.make(environment: .mock))
    }

    func testEmptyRequiredIsFlaggedAndNotSaved() async throws {
        let rt = try runtime()
        let draft = FormDraft(entity: try XCTUnwrap(rt.dsl.entity("memo")))
        let ctx = ActionContext(route: Route(screen: "form", stack: "sheet"), form: draft)

        await rt.run(Action(type: .saveRecord, entity: "memo"), ctx)

        XCTAssertEqual(draft.flaggedFields, ["body"])
        XCTAssertEqual(draft.flagCount, 1)
        XCTAssertEqual(rt.toast, L10n.tr("\("本文") を入力してください"))
        let saved = await rt.recordSync.cached(appId: appId, entity: "memo")
        XCTAssertTrue(saved.isEmpty)
    }

    func testFilledFormSavesWithoutRejection() async throws {
        let rt = try runtime()
        let draft = FormDraft(entity: try XCTUnwrap(rt.dsl.entity("memo")))
        draft.flagMissingRequired()
        draft.values["body"] = .string("牛乳")

        await rt.run(Action(type: .saveRecord, entity: "memo"), ActionContext(route: Route(screen: "form", stack: "sheet"), form: draft))

        XCTAssertEqual(draft.flagCount, 1, "保存できた時は数えない(保存バーが「保存しました」を出す)")
        XCTAssertTrue(draft.flaggedFields.isEmpty)
        let saved = await rt.recordSync.cached(appId: appId, entity: "memo")
        XCTAssertEqual(saved.count, 1)
    }
}
