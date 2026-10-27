import XCTest
import MetamonDSL
import MetamonServices
@testable import MetamonRenderer
@testable import Metamon

/// 親から子を作る(#256): navigate.prefillRef で今のレコードを子フォームの ref に積み、子は新規で開く。
@MainActor
final class PrefillRefNavigationTests: XCTestCase {
    private func runtime() throws -> AppRuntime {
        let doc = try DSLExamples.document("post_comments", version: 1)
        return AppRuntime(document: doc, services: BuildConfig.make(environment: .mock))
    }

    private func writeComment(_ rt: AppRuntime) throws -> Action {
        let form = try XCTUnwrap(rt.dsl.screen("post_form"))
        return try XCTUnwrap(form.allComponents.first { $0.id == "write_comment" }?.action)
    }

    override func tearDown() async throws {
        // 次のテストに古い初期値を残さない
        if let comments = try? DSLExamples.load("post_comments").entity("post_comments") {
            _ = FormPrefill.consume(for: comments)
        }
    }

    func testButtonOnSavedParentPrefillsRefAndOpensNewChild() async throws {
        let rt = try runtime()
        let posts = try XCTUnwrap(rt.dsl.entity("posts"))
        let draft = FormDraft(entity: posts, record: Record(id: "p1", data: ["body": .string("こんにちは")]))
        let ctx = ActionContext(route: Route(screen: "post_form", recordId: "p1", stack: "sheet"), form: draft)

        let action = try writeComment(rt)
        await rt.run(action, ctx)

        let opened = try XCTUnwrap(rt.sheet, "子のフォームがシートで開く")
        XCTAssertEqual(opened.screen, "comment_form")
        XCTAssertNil(opened.recordId, "子は新規(親の id を recordId に渡さない)")
        XCTAssertNil(rt.toast)
        let comments = try XCTUnwrap(rt.dsl.entity("post_comments"))
        XCTAssertEqual(FormPrefill.consume(for: comments), ["post": .string("p1")])
        XCTAssertEqual(FormPrefill.consume(for: comments), [:], "1 回だけ消費される")
    }

    func testRowTapPrefillsTappedRecord() async throws {
        let rt = try runtime()
        let action = Action(type: .navigate, screen: "comment_form", presentation: .sheet, prefillRef: "post")
        let ctx = ActionContext(route: Route(screen: "home", stack: Route.tabStack("home")), recordId: "p2")

        await rt.run(action, ctx)

        XCTAssertNil(rt.sheet?.recordId)
        XCTAssertEqual(rt.sheet?.screen, "comment_form")
        let comments = try XCTUnwrap(rt.dsl.entity("post_comments"))
        XCTAssertEqual(FormPrefill.consume(for: comments), ["post": .string("p2")])
    }

    func testUnsavedParentShowsToastAndStays() async throws {
        let rt = try runtime()
        let posts = try XCTUnwrap(rt.dsl.entity("posts"))
        let ctx = ActionContext(route: Route(screen: "post_form", stack: "sheet"), form: FormDraft(entity: posts))

        let action = try writeComment(rt)
        await rt.run(action, ctx)

        XCTAssertNil(rt.sheet, "親が未保存なら子のフォームを開かない")
        XCTAssertEqual(rt.toast, L10n.tr("先に「\(posts.name)」を保存してください"))
        let comments = try XCTUnwrap(rt.dsl.entity("post_comments"))
        XCTAssertEqual(FormPrefill.consume(for: comments), [:])
    }

    func testNavigateWithoutPrefillRefKeepsRecordId() async throws {
        let rt = try runtime()
        let action = Action(type: .navigate, screen: "post_form", presentation: .sheet)
        let ctx = ActionContext(route: Route(screen: "home", stack: Route.tabStack("home")), recordId: "p3")

        await rt.run(action, ctx)

        XCTAssertEqual(rt.sheet?.recordId, "p3", "従来どおり行の id で編集を開く")
        let posts = try XCTUnwrap(rt.dsl.entity("posts"))
        XCTAssertEqual(FormPrefill.consume(for: posts), [:])
    }
}
