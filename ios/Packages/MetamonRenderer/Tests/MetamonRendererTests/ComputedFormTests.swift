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
        XCTAssertNil(draft.values["perPerson"], "computed はドラフトに持たない")
        draft.values["title"] = "夕食"
        draft.values["amount"] = 3000
        draft.values["payer"] = "m1"
        draft.values["perPerson"] = 999 // 万一入っても送らない
        XCTAssertNil(draft.payload["perPerson"])
        XCTAssertEqual(draft.payload["payer"], "m1")
        XCTAssertEqual(draft.missingRequired, [])
    }

    @MainActor func testFormOverrideTargetsComputedAndRef() throws {
        let dsl = try warikan()
        let form = FormDraft(entity: try XCTUnwrap(dsl.entity("expenses")))
        let ctx = RenderContext(route: Route(screen: "expense_form"), form: form)
        let fields = try XCTUnwrap(dsl.screen("expense_form")?.components.first?.children)
        let overridden = fields.filter { FormFieldOverride.applies(to: $0, ctx: ctx) }.compactMap(\.field)
        XCTAssertEqual(Set(overridden), ["payer", "perPerson"])
        XCTAssertFalse(FormFieldOverride.applies(to: fields[0], ctx: RenderContext(route: Route(screen: "x"))), "フォーム外では差し替えない")
    }

    func testEntitiesNeededFollowsComputedAndRef() throws {
        let dsl = try warikan()
        XCTAssertEqual(dsl.entitiesNeeded(by: try XCTUnwrap(dsl.screen("home"))), ["members", "expenses"])
        // フォームは expenses だけを直接使うが、ref(payer → members)と computed(count members)で members も要る
        XCTAssertEqual(dsl.entitiesNeeded(by: try XCTUnwrap(dsl.screen("expense_form"))), ["members", "expenses"])
    }
}
