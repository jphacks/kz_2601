import XCTest
import PDFKit
import MetamonDSL
@testable import MetamonRenderer
import MetamonServices
@testable import Metamon

/// メニューの保存 ⌘S / ⌘Return(#1138)と、書き出し・プリント ⌘P(#1139)の押せる / 押せないと中身。
/// 記録はメモリ上の appconfig(`mockRecordSync`)、AI は `MockAIClient`(通信しない)。パネル・アラートは差し替える。
@MainActor
final class MacMenuCommandsTests: XCTestCase {
    private let home = Route(screen: "home", stack: Route.tabStack("home"))
    private let formSheet = Route(screen: "expense_form", stack: "sheet")
    private var originalOutput: (@MainActor () async -> MacExportOutput?)!
    private var originalDestination: (@MainActor (ExportFile) async -> URL?)!
    private var originalPrint: (@MainActor (PDFDocument) -> Bool)!
    private var outDir: URL!

    override func setUp() async throws {
        originalOutput = MacExportSaver.chooseOutput
        originalDestination = MacExportSaver.chooseDestination
        originalPrint = MacExportSaver.print
        outDir = FileManager.default.temporaryDirectory.appendingPathComponent("mac-menu-test-\(UUID().uuidString)", isDirectory: true)
        try FileManager.default.createDirectory(at: outDir, withIntermediateDirectories: true)
    }

    override func tearDown() async throws {
        MacExportSaver.chooseOutput = originalOutput
        MacExportSaver.chooseDestination = originalDestination
        MacExportSaver.print = originalPrint
        try? FileManager.default.removeItem(at: outDir)
    }

    private func runtime() throws -> MacAppRuntime {
        let url = try XCTUnwrap(Bundle(for: MacAppRuntime.self).url(forResource: "kakeibo", withExtension: "json"))
        return MacAppRuntime(appId: "test-mac-\(UUID().uuidString.lowercased())", dsl: try AppDSL.load(contentsOf: url),
                             ai: MockAIClient(delay: .zero), isAIConsentGranted: { false })
    }

    /// 支出のフォームをシートで開き、`ScreenView` と同じくフォームを控えに入れる
    private func openForm(_ rt: MacAppRuntime) async throws -> FormDraft {
        let open = try JSONDecoder().decode(Action.self, from: Data(#"{"type":"navigate","screen":"expense_form","presentation":"sheet"}"#.utf8))
        await rt.run(open, ActionContext(route: home))
        XCTAssertEqual(rt.sheet, formSheet)
        let form = FormDraft(entity: try XCTUnwrap(rt.dsl.entity("expense")))
        rt.openForms.track(form, at: formSheet)
        return form
    }

    // MARK: - 保存(#1138)

    func testSaveDisabledWithoutForm() throws {
        let rt = try runtime()
        XCTAssertNil(rt.menuSaveTarget, "一覧の画面にはフォームが無い")
        XCTAssertFalse(rt.canSaveFromMenu)
    }

    func testSaveDisabledUntilSomethingChanges() async throws {
        let rt = try runtime()
        let form = try await openForm(rt)
        XCTAssertNotNil(rt.menuSaveTarget)
        XCTAssertEqual(rt.menuSaveTarget?.action.type, .saveRecord, "保存バーと同じ saveRecord")
        XCTAssertFalse(rt.canSaveFromMenu, "開いただけでは押せない")
        form.values["memo"] = .string("コンビニ")
        XCTAssertTrue(rt.canSaveFromMenu)
    }

    func testSaveDisabledForReadOnlyFormAndWhileDialogShows() async throws {
        let rt = try runtime()
        let form = try await openForm(rt)
        form.values["memo"] = .string("コンビニ")
        rt.confirm = RuntimeConfirm(message: "?", primaryLabel: "OK", isDestructive: false) {}
        XCTAssertFalse(rt.canSaveFromMenu, "確認ダイアログの間は押せない")
        rt.confirm = nil
        form.isReadOnly = true
        XCTAssertFalse(rt.canSaveFromMenu, "他人の記録は保存バーと同じく保存できない")
    }

    func testSaveIgnoresFormThatIsNotOnTop() async throws {
        let rt = try runtime()
        let form = try await openForm(rt)
        form.values["memo"] = .string("コンビニ")
        rt.dismissTopModal()
        XCTAssertNil(rt.menuSaveTarget, "閉じた画面のフォームは保存しない")
        XCTAssertFalse(rt.canSaveFromMenu)
    }

    func testSaveFlagsRequiredFieldsLikeSaveBar() async throws {
        let rt = try runtime()
        let form = try await openForm(rt)
        form.values["memo"] = .string("金額なし") // 金額(必須)が空
        await rt.saveFromMenu()
        XCTAssertTrue(form.flaggedFields.contains("amount"), "必須の欄に印を付ける")
        XCTAssertEqual(rt.sheet, formSheet, "保存できなければ閉じない")
        XCTAssertTrue(rt.records(for: "expense").isEmpty)
    }

    func testSaveWritesRecordAndClosesForm() async throws {
        let rt = try runtime()
        let form = try await openForm(rt)
        form.values["amount"] = .number(500)
        form.values["memo"] = .string("コンビニ")
        await rt.saveFromMenu()
        XCTAssertEqual(rt.records(for: "expense").count, 1)
        XCTAssertEqual(rt.records(for: "expense").first?.data["memo"], .string("コンビニ"))
        XCTAssertNil(rt.sheet, "保存したらフォームを閉じる(保存バーと同じ)")
    }

    func testSaveActionPrefersToolbarThenComponentButton() throws {
        let toolbar = try JSONDecoder().decode(Screen.self, from: Data(#"""
        {"id":"f","entity":"e","toolbar":[{"label":"保存","action":{"type":"saveRecord","entity":"e"}}],
         "components":[{"type":"button","label":"別","action":{"type":"saveRecord","entity":"e","screen":"x"}}]}
        """#.utf8))
        XCTAssertNil(MacAppRuntime.saveAction(in: toolbar)?.screen)
        let inline = try JSONDecoder().decode(Screen.self, from: Data(#"""
        {"id":"f","entity":"e","components":[{"type":"button","label":"保存","action":{"type":"saveRecord","entity":"e","screen":"x"}}]}
        """#.utf8))
        XCTAssertEqual(MacAppRuntime.saveAction(in: inline)?.screen, "x")
        let none = try JSONDecoder().decode(Screen.self, from: Data(#"{"id":"f","components":[]}"#.utf8))
        XCTAssertNil(MacAppRuntime.saveAction(in: none))
    }

    func testRegistryReturnsLatestFormForReopenedRoute() throws {
        let entity = try XCTUnwrap(try runtime().dsl.entity("expense"))
        let registry = OpenFormRegistry()
        let old = FormDraft(entity: entity)
        let new = FormDraft(entity: entity)
        registry.track(old, at: formSheet)
        registry.track(new, at: formSheet)
        XCTAssertTrue(registry.form(at: formSheet) === new)
        registry.untrack(new)
        XCTAssertTrue(registry.form(at: formSheet) === old)
        XCTAssertNil(registry.form(at: home))
    }

    // MARK: - 書き出し・プリント(#1139)

    func testExportEnabledOnlyWithListOnTopScreen() async throws {
        let rt = try runtime()
        XCTAssertTrue(rt.canExportFromMenu, "ホームには支出の一覧がある")
        let csv = try XCTUnwrap(rt.menuExportAction(format: "csv"))
        XCTAssertEqual(csv.type, .export)
        XCTAssertEqual(csv.entity, "expense")
        XCTAssertEqual(csv.format, "csv")

        _ = try await openForm(rt)
        XCTAssertFalse(rt.canExportFromMenu, "フォーム(一覧なし)のシートでは押せない")
        rt.dismissTopModal()
        rt.selectedTab = "stats"
        XCTAssertFalse(rt.canExportFromMenu, "グラフだけの画面では押せない")
    }

    func testExportOpensOutputChoiceAndSaves() async throws {
        let rt = try runtime()
        var asked = 0
        MacExportSaver.chooseOutput = {
            asked += 1
            return .save
        }
        let dest = outDir.appendingPathComponent("out.pdf")
        MacExportSaver.chooseDestination = { file in
            XCTAssertEqual(file.url.pathExtension, "pdf")
            return dest
        }
        await rt.exportFromMenu(format: "pdf")
        XCTAssertEqual(asked, 1, "export と同じく保存・共有・プリントを選ぶ")
        XCTAssertTrue(FileManager.default.fileExists(atPath: dest.path))
        XCTAssertEqual(rt.toast, L10n.tr("書き出しました"))
    }

    func testPrintSendsListToPrintPanelWithoutChoice() async throws {
        let rt = try runtime()
        MacExportSaver.chooseOutput = {
            XCTFail("⌘P は出し方を聞かずにプリントする")
            return nil
        }
        var printed = 0
        MacExportSaver.print = { _ in
            printed += 1
            return true
        }
        await rt.printFromMenu()
        XCTAssertEqual(printed, 1)
        XCTAssertNil(rt.toast)
    }

    func testPrintDoesNothingWithoutList() async throws {
        let rt = try runtime()
        rt.selectedTab = "stats"
        MacExportSaver.print = { _ in
            XCTFail("一覧が無ければプリントしない")
            return true
        }
        await rt.printFromMenu()
    }
}
