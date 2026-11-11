import XCTest
import MetamonDSL
@testable import MetamonRenderer
@testable import Metamon

/// #1719: タブ / ホームの根に置いたフォームで保存すると、閉じる先が無くても「保存しました」を出し、新規なら入力を空に戻す。
@MainActor
final class RootFormSaveTests: XCTestCase {
    private let appId = "test-rootsave-\(UUID().uuidString.lowercased())"

    private func runtime() throws -> AppRuntime {
        let json = """
        {"schemaVersion":1,"app":{"name":"メモ","voiceEntry":"floating",
          "tabs":[{"screen":"form","title":"入力","icon":"pencil"},{"screen":"list","title":"一覧","icon":"list.bullet"}]},
         "initialScreen":"form",
         "entities":[{"id":"memo","name":"メモ","fields":[{"id":"body","name":"本文","type":"text","required":true}]}],
         "screens":[{"id":"form","title":"メモ","entity":"memo","components":[{"type":"textField","field":"body"}]},
                    {"id":"list","title":"一覧","components":[{"type":"list","entity":"memo","titleField":"body"}]}]}
        """
        let dsl = try JSONDecoder().decode(AppDSL.self, from: Data(json.utf8))
        return AppRuntime(document: AppDocument(appId: appId, version: 1, dsl: dsl), services: BuildConfig.make(environment: .mock))
    }

    private var tabRoot: Route { Route(screen: "form", stack: Route.tabStack("form")) }

    private func save(_ rt: AppRuntime, _ draft: FormDraft, at route: Route) async {
        await rt.run(Action(type: .saveRecord, entity: "memo"), ActionContext(route: route, form: draft))
    }

    func testNewRecordOnTabRootResetsFormAndToasts() async throws {
        let rt = try runtime()
        rt.selectedTab = "form"
        let draft = FormDraft(entity: try XCTUnwrap(rt.dsl.entity("memo")))
        draft.values["body"] = .string("散歩した")

        await save(rt, draft, at: tabRoot)

        XCTAssertEqual(rt.toast, L10n.tr("保存しました"))
        XCTAssertNil(draft.payload["body"], "新規は入力を空に戻す")
        XCTAssertFalse(draft.hasUnsavedChanges)
        XCTAssertFalse(draft.isEditing)
        XCTAssertEqual(draft.saveCount, 1, "保存バーも「保存しました」を出す")
        XCTAssertEqual(rt.selectedTab, "form", "一覧のタブへは切り替えない")
        let saved = await rt.recordSync.cached(appId: appId, entity: "memo")
        XCTAssertEqual(saved.count, 1)
    }

    func testEditOnTabRootKeepsValues() async throws {
        let rt = try runtime()
        let entity = try XCTUnwrap(rt.dsl.entity("memo"))
        let first = FormDraft(entity: entity)
        first.values["body"] = .string("牛乳")
        await save(rt, first, at: tabRoot)
        rt.toast = nil

        let saved = await rt.recordSync.cached(appId: appId, entity: "memo")
        let editing = FormDraft(entity: entity, record: try XCTUnwrap(saved.first))
        editing.values["body"] = .string("豆乳")
        await save(rt, editing, at: tabRoot)
        XCTAssertEqual(rt.toast, L10n.tr("保存しました"))
        XCTAssertEqual(editing.payload["body"], .string("豆乳"), "既存の記録は値を残す")

        rt.toast = nil
        await save(rt, editing, at: tabRoot)
        XCTAssertNil(rt.toast, "何も変えずに保存したら何も出さない(M10)")
    }

    func testPushedFormStillCloses() async throws {
        let rt = try runtime()
        let stack = Route.tabStack("list")
        let pushed = Route(screen: "form", stack: stack)
        rt.paths[stack] = [pushed]
        let draft = FormDraft(entity: try XCTUnwrap(rt.dsl.entity("memo")))
        draft.values["body"] = .string("メモ")

        await save(rt, draft, at: pushed)

        XCTAssertEqual(rt.paths[stack], [], "積まれたフォームは閉じる")
        XCTAssertNil(rt.toast, "閉じる時は toast を出さない(今までどおり)")
    }
}
