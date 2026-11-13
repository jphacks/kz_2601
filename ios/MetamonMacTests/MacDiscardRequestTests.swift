import XCTest
import MetamonDSL
@testable import MetamonRenderer
import MetamonServices
@testable import Metamon

/// 保存していない入力の破棄の要求(`DiscardRequest`・`MacAppRuntime+Discard`・#2853)。
/// Esc(閉じる)・⌘[(戻る)・アプリの切り替え(`confirmDiscard`)の確認の元。ウインドウの見張りは MacDiscardGuardTests。
/// 「破棄」= `discardRequest.perform()`、「やめる」= `discardRequest = nil`(`AppCanvas` のダイアログと同じ)。
@MainActor
final class MacDiscardRequestTests: XCTestCase {
    private let home = Route(screen: "home", stack: Route.tabStack("home"))
    private let formSheet = Route(screen: "expense_form", stack: "sheet")

    /// 控えはフォームを弱く持つので、テストの間はここで持つ(`ScreenView` が持つ代わり)
    private var forms: [FormDraft] = []

    override func tearDown() async throws {
        forms = []
    }

    private func runtime() throws -> MacAppRuntime {
        let url = try XCTUnwrap(Bundle(for: MacAppRuntime.self).url(forResource: "kakeibo", withExtension: "json"))
        return MacAppRuntime(appId: "test-mac-\(UUID().uuidString.lowercased())", dsl: try AppDSL.load(contentsOf: url),
                             ai: MockAIClient(delay: .zero), isAIConsentGranted: { false })
    }

    /// 支出のフォームをシートで開き、`ScreenView` と同じく控えに入れる。`memo` があれば入力する
    private func openForm(_ rt: MacAppRuntime, memo: String? = nil) async throws {
        let open = try JSONDecoder().decode(Action.self, from: Data(#"{"type":"navigate","screen":"expense_form","presentation":"sheet"}"#.utf8))
        await rt.run(open, ActionContext(route: home))
        XCTAssertEqual(rt.sheet, formSheet)
        let form = FormDraft(entity: try XCTUnwrap(rt.dsl.entity("expense")))
        if let memo { form.values["memo"] = .string(memo) }
        rt.openForms.track(form, at: formSheet)
        forms.append(form)
    }

    // MARK: - 要求の作り方

    func testRequestsHaveDistinctIds() {
        let a = DiscardRequest {}
        let b = DiscardRequest {}
        XCTAssertNotEqual(a.id, b.id, "同じ操作でも別の要求(ダイアログを出し直せる)")
    }

    func testConfirmDiscardRunsAtOnceWithoutInput() async throws {
        let rt = try runtime()
        try await openForm(rt) // 開いただけ
        var done = false
        rt.confirmDiscard { done = true }
        XCTAssertTrue(done, "失う入力が無ければすぐ進める")
        XCTAssertNil(rt.discardRequest)
    }

    func testConfirmDiscardAsksWithInput() async throws {
        let rt = try runtime()
        try await openForm(rt, memo: "コンビニ")
        var done = false
        rt.confirmDiscard { done = true }
        XCTAssertFalse(done, "確認の答えを待つ")
        XCTAssertNotNil(rt.discardRequest)
    }

    func testLeavingOtherStackDoesNotAsk() async throws {
        let rt = try runtime()
        try await openForm(rt, memo: "コンビニ")
        XCTAssertFalse(rt.hasUnsavedInput(leaving: { $0.stack == "full" }), "閉じない画面の入力は数えない")
        var done = false
        rt.confirmDiscard(leaving: { $0.stack == "full" }) { done = true }
        XCTAssertTrue(done)
        XCTAssertNil(rt.discardRequest)
    }

    // MARK: - 閉じる(Esc)

    func testDismissTopModalAsksThenClosesOnDiscard() async throws {
        let rt = try runtime()
        try await openForm(rt, memo: "コンビニ")
        rt.requestDismissTopModal()
        XCTAssertNotNil(rt.sheet, "確認の前には閉じない")
        let request = try XCTUnwrap(rt.discardRequest)
        rt.discardRequest = nil
        request.perform() // 破棄
        XCTAssertNil(rt.sheet, "「破棄」でシートを閉じる")
    }

    func testDismissTopModalClosesAtOnceWithoutInput() async throws {
        let rt = try runtime()
        try await openForm(rt)
        rt.requestDismissTopModal()
        XCTAssertNil(rt.discardRequest)
        XCTAssertNil(rt.sheet)
    }

    // MARK: - 戻る(⌘[)

    func testGoBackAsksThenGoesBackOnDiscard() async throws {
        let rt = try runtime()
        try await openForm(rt, memo: "コンビニ")
        rt.requestGoBack()
        XCTAssertNotNil(rt.sheet)
        let request = try XCTUnwrap(rt.discardRequest)
        rt.discardRequest = nil
        request.perform()
        XCTAssertNil(rt.sheet, "シートの最下層なので、戻るとシートごと閉じる")
    }

    func testGoBackDoesNothingWhenNothingToGoBack() throws {
        let rt = try runtime()
        XCTAssertFalse(rt.canGoBack)
        rt.requestGoBack()
        XCTAssertNil(rt.discardRequest)
    }

    // MARK: - 取り消し(やめる)

    func testCancelKeepsSheetAndInput() async throws {
        let rt = try runtime()
        try await openForm(rt, memo: "コンビニ")
        rt.requestDismissTopModal()
        XCTAssertNotNil(rt.discardRequest)
        rt.discardRequest = nil // やめる(perform は呼ばない)
        XCTAssertEqual(rt.sheet, formSheet, "「やめる」では閉じない")
        XCTAssertTrue(rt.hasUnsavedInput(), "入力は残っている")
    }

    func testAskingAgainAfterCancelMakesNewRequest() async throws {
        let rt = try runtime()
        try await openForm(rt, memo: "コンビニ")
        rt.requestDismissTopModal()
        let first = try XCTUnwrap(rt.discardRequest).id
        rt.discardRequest = nil
        rt.requestDismissTopModal()
        let second = try XCTUnwrap(rt.discardRequest).id
        XCTAssertNotEqual(first, second)
    }
}
