import XCTest
import MetamonDSL
@testable import MetamonRenderer
@testable import Metamon

/// #1719: タブ / ホームの根に置いたフォームで保存すると、閉じる先が無くても「保存しました」を出し、新規なら入力を空に戻す。
@MainActor
final class RootFormSaveTests: XCTestCase {
    private let appId = "test-rootsave-\(UUID().uuidString.lowercased())"

    private func runtime(single: Bool = false) throws -> AppRuntime {
        let json = """
        {"schemaVersion":1,"app":{"name":"メモ","voiceEntry":"floating",
          "tabs":[{"screen":"form","title":"入力","icon":"pencil"},{"screen":"list","title":"一覧","icon":"list.bullet"}]},
         "initialScreen":"form",
         "entities":[{"id":"memo","name":"メモ",\(single ? "\"single\":true," : "")"fields":[{"id":"body","name":"本文","type":"text","required":true}]}],
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

    /// 3 秒以内に 2 回保存: 文言は同じでも toastSerial が変わり、ToastHost の表示時間が出し直しになる。VoiceOver も 2 回読む
    func testSecondSaveWithSameToastRestartsToast() async throws {
        let original = VoiceOverAnnouncer.post
        defer { VoiceOverAnnouncer.post = original }
        var posted: [String] = []
        VoiceOverAnnouncer.post = { posted.append($0) }
        let rt = try runtime()
        let draft = FormDraft(entity: try XCTUnwrap(rt.dsl.entity("memo")))
        draft.values["body"] = .string("1")
        await save(rt, draft, at: tabRoot)
        let first = rt.toastSerial

        draft.values["body"] = .string("2")
        await save(rt, draft, at: tabRoot)

        let saved = await rt.recordSync.cached(appId: appId, entity: "memo")
        XCTAssertEqual(saved.count, 2)
        XCTAssertEqual(rt.toast, L10n.tr("保存しました"))
        XCTAssertNotEqual(rt.toastSerial, first, "同じ文言でも .task(id:) を出し直す")
        XCTAssertEqual(posted, [L10n.tr("保存しました"), L10n.tr("保存しました")], "2 回目も読む")
    }

    /// 保存の間に入れ直した値は、保存の後で空に戻さない
    func testTypingDuringSaveIsKept() async throws {
        let rt = try runtime()
        let draft = FormDraft(entity: try XCTUnwrap(rt.dsl.entity("memo")))
        draft.values["body"] = .string("買い物")

        let saving = Task { await save(rt, draft, at: tabRoot) }
        // 書き込み(recordSync の actor)を待っている間に次の入力が入る。main actor で見えた時は必ず await の途中
        for _ in 0..<1000 where !rt.saveFlight.isSaving(draft) { await Task.yield() }
        XCTAssertTrue(rt.saveFlight.isSaving(draft), "保存の途中に入れる")
        draft.values["body"] = .string("次のメモ")
        await saving.value

        let saved = await rt.recordSync.cached(appId: appId, entity: "memo")
        XCTAssertEqual(saved.count, 1)
        XCTAssertEqual(saved.first?.data["body"], .string("買い物"), "保存したのは押した時の値")
        XCTAssertEqual(draft.payload["body"], .string("次のメモ"), "入れ直した値を消さない")
        XCTAssertEqual(rt.toast, L10n.tr("保存しました"))
    }

    /// single(#267)は 1 件を上書きし続ける。最初の保存(まだ記録が無い)でも入力を空に戻さない。
    func testSingleEntityKeepsValuesOnFirstSave() async throws {
        let rt = try runtime(single: true)
        let draft = FormDraft(entity: try XCTUnwrap(rt.dsl.entity("memo")))
        draft.values["body"] = .string("目標 10km")

        await save(rt, draft, at: tabRoot)

        XCTAssertEqual(rt.toast, L10n.tr("保存しました"))
        XCTAssertEqual(draft.payload["body"], .string("目標 10km"), "single は値を残す")
    }

    /// 空に戻した後にサーバーが断ったら、送った入力をフォームに戻す(打ち直さなくてよい)。
    func testRefusedSaveRestoresResetForm() async throws {
        let rt = try runtime()
        let draft = FormDraft(entity: try XCTUnwrap(rt.dsl.entity("memo")))
        draft.values["body"] = .string("散歩した")
        await save(rt, draft, at: tabRoot)
        XCTAssertNil(draft.payload["body"])
        let saved = try XCTUnwrap(rt.lastSavedForm)

        rt.revokeSaveFeedback(for: .rejected(appId: appId, entity: "memo", message: "bad", recordId: saved.recordId))

        XCTAssertEqual(draft.payload["body"], .string("散歩した"), "送った入力を戻す")
        XCTAssertTrue(draft.hasUnsavedChanges, "未保存に戻る(閉じる前に破棄の確認が出る)")
        XCTAssertNil(draft.lastSaveResult)
    }

    /// 空に戻した後に次の入力を始めていたら、断られても上書きしない。
    func testRefusedSaveKeepsNewInput() async throws {
        let rt = try runtime()
        let draft = FormDraft(entity: try XCTUnwrap(rt.dsl.entity("memo")))
        draft.values["body"] = .string("散歩した")
        await save(rt, draft, at: tabRoot)
        let saved = try XCTUnwrap(rt.lastSavedForm)
        draft.values["body"] = .string("次のメモ")

        rt.revokeSaveFeedback(for: .rejected(appId: appId, entity: "memo", message: "bad", recordId: saved.recordId))

        XCTAssertEqual(draft.payload["body"], .string("次のメモ"))
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
