import XCTest
import MetamonDSL
import MetamonRenderer
import MetamonServices
@testable import Metamon

/// 別の生成アプリのフォームを値入りで開く本体の口(#2235): フォームを探す(1 つ・複数・無い)・合う項目だけ入る・保存した時だけ戻る。
@MainActor
final class OpenForm2235Tests: XCTestCase {
    private let json = """
    {"schemaVersion":1,"app":{"name":"家計簿"},"initialScreen":"home",
     "entities":[{"id":"expense","name":"支出","fields":[
                    {"id":"amount","name":"金額","type":"integer"},
                    {"id":"title","name":"品目","type":"string"},
                    {"id":"day","name":"日付","type":"date"},
                    {"id":"kind","name":"種類","type":"enum","options":["食費","日用品"]},
                    {"id":"secret","name":"管理メモ","type":"string","editableBy":"admins"},
                    {"id":"photo","name":"写真","type":"photo"}]},
                 {"id":"memo","name":"メモ","fields":[{"id":"text","name":"内容","type":"string"}]},
                 {"id":"todo","name":"やること","fields":[{"id":"text","name":"内容","type":"string"}]}],
     "screens":[{"id":"home","title":"家計簿","components":[
                  {"type":"list","entity":"expense","item":{"title":"title"}},
                  {"type":"button","label":"追加","action":{"type":"navigate","screen":"add","presentation":"sheet"}},
                  {"type":"button","label":"やること","action":{"type":"navigate","screen":"todoA","presentation":"sheet"}}]},
                {"id":"add","title":"支出を追加","entity":"expense","components":[{"type":"numberField","field":"amount"}]},
                {"id":"todoA","title":"やること","entity":"todo","components":[{"type":"textField","field":"text"}]},
                {"id":"todoB","title":"やること(別)","entity":"todo","components":[{"type":"textField","field":"text"}]}]}
    """

    private func dsl() throws -> AppDSL { try JSONDecoder().decode(AppDSL.self, from: Data(json.utf8)) }

    // MARK: フォームを探す

    func testFindsTheOnlyForm() throws {
        XCTAssertEqual(OpenFormPrefill.formScreens(for: "expense", in: try dsl()), ["add"])
        guard case .success(let plan) = OpenFormPrefill.plan(entityId: "expense", values: [:], in: try dsl()) else { return XCTFail() }
        XCTAssertEqual(plan.screen, "add")
    }

    func testMultipleFormsUseTheFirstInDSL() throws {
        XCTAssertEqual(OpenFormPrefill.formScreens(for: "todo", in: try dsl()), ["todoA", "todoB"])
        guard case .success(let plan) = OpenFormPrefill.plan(entityId: "todo", values: [:], in: try dsl()) else { return XCTFail() }
        XCTAssertEqual(plan.screen, "todoA")
    }

    func testNoFormOrNoEntityFails() throws {
        XCTAssertEqual(OpenFormPrefill.plan(entityId: "memo", values: [:], in: try dsl()), .failure(.noForm))
        XCTAssertEqual(OpenFormPrefill.plan(entityId: "nope", values: [:], in: try dsl()), .failure(.noEntity))
    }

    // MARK: 合う項目だけ入る

    func testOnlyMatchingFieldsAreFilled() throws {
        let values: [String: JSONValue] = [
            "amount": .number(1200),          // id で合う
            "品目": .string("牛乳"),           // 名前で合う
            "種類": .string("外食"),           // 選択肢に無い
            "日付": .string("2026-11-13"),
            "photo": .string("file:///x.jpg"), // 写真は入れない
            "secret": .string("x"),            // 管理者だけの項目
            "unknown": .number(1),
        ]
        guard case .success(let plan) = OpenFormPrefill.plan(entityId: "expense", values: values, in: try dsl()) else { return XCTFail() }
        XCTAssertEqual(plan.values["amount"], .number(1200))
        XCTAssertEqual(plan.values["title"], .string("牛乳"))
        XCTAssertEqual(plan.values["day"], .string("2026-11-13"))
        XCTAssertEqual(Set(plan.values.keys), ["amount", "title", "day"])
        XCTAssertEqual(plan.skipped, ["photo", "secret", "unknown", "種類"])
    }

    func testAdminFillsAdminOnlyAndTypesAreCoerced() throws {
        let values: [String: JSONValue] = ["secret": .string("x"), "金額": .string("980"), "amount": .number(1.5)]
        guard case .success(let plan) = OpenFormPrefill.plan(entityId: "expense", values: values, in: try dsl(), isAdmin: true) else { return XCTFail() }
        XCTAssertEqual(plan.values["secret"], .string("x"))
        // 小数は integer に入らない → id の値は落ち、名前の「金額」(数字の文字)が入る
        XCTAssertEqual(plan.values["amount"], .number(980))
        XCTAssertEqual(plan.skipped, ["amount"])
    }

    // MARK: 本体の口

    private func runtime() throws -> AppRuntime {
        let services = BuildConfig.make(environment: .mock)
        return AppRuntime(document: AppDocument(appId: "kakeibo-\(UUID().uuidString.prefix(6))", version: 1, dsl: try dsl()), services: services)
    }

    func testOpenFormOnCurrentAppOpensTheFormWithoutReturn() async throws {
        let rt = try runtime()
        let outcome = await rt.openForm(appId: rt.document.appId, entityId: "expense", values: ["amount": .number(500), "x": .number(1)])
        XCTAssertEqual(outcome, .opened(skipped: ["x"]))
        XCTAssertEqual(NavigationResolver.top(rt.navigationState, dsl: rt.dsl).screen, "add")
        XCTAssertNil(rt.openFormReturn) // 同じアプリなら戻る先は無い
        XCTAssertNotNil(rt.toast)
    }

    func testOpenFormWithoutFormToasts() async throws {
        let rt = try runtime()
        let outcome = await rt.openForm(appId: rt.document.appId, entityId: "memo", values: [:])
        XCTAssertEqual(outcome, .noForm)
        XCTAssertNotNil(rt.toast)
    }

    func testReturnIsTakenOnlyForTheNewRecordOfThatFormOnce() throws {
        let rt = try runtime()
        let now = Date()
        rt.openFormReturn = .init(originAppId: "shopping", appId: "kakeibo", entityId: "expense", screen: "add", openedAt: now)
        XCTAssertNil(rt.takeOpenFormReturn(appId: "kakeibo", entityId: "expense", route: Route(screen: "add", recordId: "r1"))) // 編集
        XCTAssertNil(rt.takeOpenFormReturn(appId: "kakeibo", entityId: "todo", route: Route(screen: "add")))
        XCTAssertNil(rt.takeOpenFormReturn(appId: "other", entityId: "expense", route: Route(screen: "add")))
        XCTAssertEqual(rt.takeOpenFormReturn(appId: "kakeibo", entityId: "expense", route: Route(screen: "add", stack: "sheet"))?.originAppId, "shopping")
        XCTAssertNil(rt.openFormReturn) // 1 回だけ
    }

    func testReturnExpires() {
        let r = AppRuntime.OpenFormReturn(originAppId: "a", appId: "b", entityId: "e", screen: "s", openedAt: Date(timeIntervalSince1970: 0))
        XCTAssertTrue(r.matches(appId: "b", entityId: "e", route: Route(screen: "s"), now: Date(timeIntervalSince1970: 60)))
        XCTAssertFalse(r.matches(appId: "b", entityId: "e", route: Route(screen: "s"), now: Date(timeIntervalSince1970: 31 * 60)))
    }
}
