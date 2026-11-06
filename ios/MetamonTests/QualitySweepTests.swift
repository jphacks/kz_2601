import XCTest
import MetamonDSL
import MetamonServices
@testable import MetamonRenderer
@testable import Metamon

/// iOS の品質の見直しで直した不具合の回帰テスト(保存の連打・生成中のアプリ切替・自動削除の注記)。
@MainActor
final class QualitySweepTests: XCTestCase {
    private let appId = "test-sweep-\(UUID().uuidString.lowercased())"

    private func runtime(retention: Bool = false, scope: String? = nil) throws -> AppRuntime {
        let extra = (retention ? #","retentionDays":7"# : "") + (scope.map { #","scope":"\#($0)""# } ?? "")
        let json = """
        {"schemaVersion":1,"app":{"name":"メモ","voiceEntry":"floating"},"initialScreen":"home",
         "entities":[{"id":"memo","name":"メモ"\(extra),"fields":[{"id":"body","name":"本文","type":"text"}]}],
         "screens":[{"id":"home","title":"メモ","components":[{"type":"list","entity":"memo"}]},
                    {"id":"form","title":"メモ","entity":"memo","components":[{"type":"textField","field":"body"}]}]}
        """
        let dsl = try JSONDecoder().decode(AppDSL.self, from: Data(json.utf8))
        return AppRuntime(document: AppDocument(appId: appId, version: 1, dsl: dsl), services: BuildConfig.make(environment: .mock))
    }

    // MARK: - 保存の連打

    func testDoubleTapSaveCreatesOneRecord() async throws {
        let rt = try runtime()
        let memo = try XCTUnwrap(rt.dsl.entity("memo"))
        let draft = FormDraft(entity: memo)
        draft.values["body"] = .string("牛乳を買う")
        let ctx = ActionContext(route: Route(screen: "form", stack: "sheet"), form: draft)
        let save = Action(type: .saveRecord, entity: "memo")

        // 同じフレームで 2 回押された
        async let first: Void = rt.run(save, ctx)
        async let second: Void = rt.run(save, ctx)
        _ = await (first, second)

        let saved = await rt.recordSync.cached(appId: appId, entity: "memo")
        XCTAssertEqual(saved.count, 1, "連打しても新規の記録は 1 件だけ")

        // 連打の後でも、もう一度押せば普通に保存できる(保存中の印が残らない)
        let next = FormDraft(entity: memo)
        next.values["body"] = .string("卵を買う")
        await rt.run(save, ActionContext(route: Route(screen: "form", stack: "sheet"), form: next))
        let after = await rt.recordSync.cached(appId: appId, entity: "memo")
        XCTAssertEqual(after.count, 2)
    }

    // MARK: - 生成中に初期状態へ戻した

    func testMorphResultIsDroppedWhenAppChangedWhileGenerating() async throws {
        let ai = UndoRedoTests.GatedAI()
        var services = BuildConfig.make(environment: .mock)
        services.ai = ai
        let rt = AppRuntime(document: try DSLExamples.document("kakeibo", version: 1), services: services)
        rt.clarifyEnabled = false
        ai.holdNextMorph()
        let job = Task { await rt.generate("金額を大きくして") }
        var started = ai.started.makeAsyncIterator()
        _ = await started.next()
        XCTAssertTrue(rt.isMorphing)

        rt.resetToInitial() // 生成の途中で初期状態へ(新しい appId)
        let freshId = rt.document.appId
        ai.releaseMorph()
        await job.value

        XCTAssertTrue(rt.dsl.isVoiceOnly, "前のアプリの生成結果で初期画面を上書きしない")
        XCTAssertEqual(rt.document.appId, freshId)
        XCTAssertNil(rt.morphJob)
        XCTAssertNil(rt.errorCard)
    }

    // MARK: - 自動削除の注記

    func testRetentionFooterMentionsSharedOnlyForSharedEntities() throws {
        let shared = L10n.tr("共有の記録は自分が追加したものだけが消えます。")
        let privateRt = try runtime(retention: true)
        XCTAssertFalse(RetentionSettingsSection.footer(entities: privateRt.dsl.entities).contains(shared),
                       "自分だけの記録(審査用の版は全部これ)に共有の説明を出さない")
        let sharedRt = try runtime(retention: true, scope: "shared")
        XCTAssertTrue(RetentionSettingsSection.footer(entities: sharedRt.dsl.entities).contains(shared))
    }
}
