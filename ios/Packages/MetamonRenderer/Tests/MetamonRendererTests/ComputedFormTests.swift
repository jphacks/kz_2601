import XCTest
import MetamonDSL
@testable import MetamonRenderer

/// #101: computed はドラフト・送信に入らない / ref と computed の部品は型で差し替わる。
final class ComputedFormTests: XCTestCase {
    private func warikan() throws -> AppDSL {
        var url = URL(fileURLWithPath: #filePath)
        for _ in 0..<6 { url.deleteLastPathComponent() }
        return try AppDSL.load(contentsOf: url.appending(path: "docs/dsl/examples/warikan.json"))
    }

    @MainActor func testComputedIsNotSaved() throws {
        let entity = try XCTUnwrap(try warikan().entity("expenses"))
        let draft = FormDraft(entity: entity)
        XCTAssertNil(draft.values["per_person"], "computed はドラフトに持たない")
        draft.values["title"] = "夕食"
        draft.values["amount"] = 3000
        draft.values["payer"] = "m1"
        draft.values["per_person"] = 999 // 万一入っても送らない
        XCTAssertNil(draft.payload["per_person"])
        XCTAssertEqual(draft.payload["payer"], "m1")
        XCTAssertEqual(draft.missingRequired, [])
    }

    @MainActor func testFormOverrideTargetsComputedAndRef() throws {
        let dsl = try warikan()
        let form = FormDraft(entity: try XCTUnwrap(dsl.entity("expenses")))
        let ctx = RenderContext(route: Route(screen: "expense_form"), form: form)
        let fields = try XCTUnwrap(dsl.screen("expense_form")?.components.first?.children)
        let overridden = fields.filter { FormFieldOverride.applies(to: $0, ctx: ctx) }.compactMap(\.field)
        XCTAssertEqual(Set(overridden), ["payer"], "正典 warikan はフォームに computed を束縛しない(backend がエラーにする)")
        // それでも AI が computed に入力部品を置いた場合は読み取り専用行に差し替える(防御)
        let stray = try JSONDecoder().decode(Component.self, from: Data(#"{"type":"numberField","field":"per_person","label":"1人あたり"}"#.utf8))
        XCTAssertTrue(FormFieldOverride.applies(to: stray, ctx: ctx))
        XCTAssertFalse(FormFieldOverride.applies(to: fields[0], ctx: RenderContext(route: Route(screen: "x"))), "フォーム外では差し替えない")
    }

    func testEntitiesNeededFollowsComputedAndRef() throws {
        let dsl = try warikan()
        XCTAssertEqual(dsl.entitiesNeeded(by: try XCTUnwrap(dsl.screen("home"))), ["members", "expenses"])
        // フォームは expenses だけを直接使うが、ref(payer → members)と computed(count members)で members も要る
        XCTAssertEqual(dsl.entitiesNeeded(by: try XCTUnwrap(dsl.screen("expense_form"))), ["members", "expenses"])
    }
}
