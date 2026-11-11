import XCTest
import MetamonDSL
@testable import MetamonRenderer
import MetamonServices
@testable import Metamon

/// Mac の中央のランタイム(`MacAppRuntime`・#1212): 画面の移動・確認・一言(toast)・同期の知らせ。
/// 記録はメモリ上の appconfig(`mockRecordSync`)、AI は `MockAIClient`(通信しない)。
@MainActor
final class MacAppRuntimeTests: XCTestCase {
    private let home = Route(screen: "home", stack: Route.tabStack("home"))

    private func kakeibo() throws -> AppDSL {
        let url = try XCTUnwrap(Bundle(for: MacAppRuntime.self).url(forResource: "kakeibo", withExtension: "json"),
                                "docs/dsl/examples の kakeibo.json がアプリに入っていない")
        return try AppDSL.load(contentsOf: url)
    }

    private func runtime() throws -> MacAppRuntime {
        MacAppRuntime(appId: "test-mac-\(UUID().uuidString.lowercased())", dsl: try kakeibo(),
                      ai: MockAIClient(delay: .zero), isAIConsentGranted: { false })
    }

    private func action(_ json: String) throws -> Action {
        try JSONDecoder().decode(Action.self, from: Data(json.utf8))
    }

    /// 同期の知らせは Task で処理されるので、条件が揃うまで少し待つ
    private func eventually(_ message: String, _ condition: () -> Bool) async {
        for _ in 0..<100 {
            if condition() { return }
            try? await Task.sleep(for: .milliseconds(20))
        }
        XCTFail(message)
    }

    // MARK: - 画面の移動

    func testStartsOnInitialTabWithNothingToGoBackTo() throws {
        let rt = try runtime()
        XCTAssertEqual(rt.selectedTab, "home")
        XCTAssertEqual(rt.topStack, Route.tabStack("home"))
        XCTAssertFalse(rt.canGoBack)
        XCTAssertEqual(rt.currentScreen, "home")
    }

    func testPushThenGoBack() async throws {
        let rt = try runtime()
        await rt.run(try action(#"{"type":"navigate","screen":"expense_form","presentation":"push"}"#),
                     ActionContext(route: home, recordId: "r1"))
        XCTAssertEqual(rt.paths[Route.tabStack("home")]?.map(\.screen), ["expense_form"])
        XCTAssertEqual(rt.paths[Route.tabStack("home")]?.first?.recordId, "r1", "押した行の記録を開く")
        XCTAssertTrue(rt.canGoBack)
        XCTAssertEqual(rt.currentScreen, "expense_form")

        rt.goBack()
        XCTAssertEqual(rt.paths[Route.tabStack("home")] ?? [], [])
        XCTAssertFalse(rt.canGoBack)
        XCTAssertEqual(rt.currentScreen, "home")
    }

    func testSheetIsTopAndEscClosesIt() async throws {
        let rt = try runtime()
        await rt.run(try action(#"{"type":"navigate","screen":"expense_form","presentation":"sheet"}"#),
                     ActionContext(route: home))
        XCTAssertEqual(rt.sheet?.screen, "expense_form")
        XCTAssertEqual(rt.topStack, "sheet")
        XCTAssertTrue(rt.canGoBack)

        rt.dismissTopModal()
        XCTAssertNil(rt.sheet)
        XCTAssertNil(rt.paths["sheet"])
        XCTAssertEqual(rt.topStack, Route.tabStack("home"))
    }

    func testBackActionClosesSheet() async throws {
        let rt = try runtime()
        await rt.run(try action(#"{"type":"navigate","screen":"expense_form","presentation":"sheet"}"#),
                     ActionContext(route: home))
        let sheetRoute = try XCTUnwrap(rt.sheet)
        await rt.run(try action(#"{"type":"back"}"#), ActionContext(route: sheetRoute))
        XCTAssertNil(rt.sheet)
    }

    func testNavigateToTabRootSwitchesTab() async throws {
        let rt = try runtime()
        await rt.run(try action(#"{"type":"navigate","screen":"stats"}"#), ActionContext(route: home))
        XCTAssertEqual(rt.selectedTab, "stats")
        XCTAssertEqual(rt.topStack, Route.tabStack("stats"))
        XCTAssertEqual(rt.currentScreen, "stats")
    }

    func testRevealFromScreenMapClosesModalAndOpens() async throws {
        let rt = try runtime()
        await rt.run(try action(#"{"type":"navigate","screen":"expense_form","presentation":"sheet"}"#),
                     ActionContext(route: home))
        rt.reveal(screen: "stats")
        XCTAssertNil(rt.sheet)
        XCTAssertEqual(rt.currentScreen, "stats")
    }

    // MARK: - 確認

    func testConfirmMessageAsksBeforeRunning() async throws {
        let rt = try runtime()
        await rt.run(try action(#"{"type":"navigate","screen":"expense_form","presentation":"sheet","confirmMessage":"開きますか？"}"#),
                     ActionContext(route: home))
        let confirm = try XCTUnwrap(rt.confirm, "先に確認を出す")
        XCTAssertNil(rt.sheet, "確認の前は動かさない")
        XCTAssertEqual(confirm.message, "開きますか？")
        XCTAssertEqual(confirm.primaryLabel, RuntimeConfirm.button(for: .navigate).label)
        XCTAssertFalse(confirm.isDestructive)

        await confirm.perform()
        XCTAssertEqual(rt.sheet?.screen, "expense_form")
    }

    func testDeleteConfirmIsDestructive() async throws {
        let rt = try runtime()
        await rt.run(try action(#"{"type":"deleteRecord","entity":"expense","confirmMessage":"この支出を削除しますか？"}"#),
                     ActionContext(route: home, recordId: "r1"))
        XCTAssertEqual(rt.confirm?.isDestructive, true)
        XCTAssertEqual(rt.confirm?.primaryLabel, RuntimeConfirm.button(for: .deleteRecord).label)
    }

    // MARK: - 一言(toast)

    func testUnsupportedActionShowsToast() async throws {
        let rt = try runtime()
        await rt.run(try action(#"{"type":"follow"}"#), ActionContext(route: home))
        XCTAssertEqual(rt.toast, L10n.tr("このボタンは Mac ではまだ使えません"))
    }

    func testOpenUrlRejectsPlainHttp() async throws {
        let rt = try runtime()
        await rt.run(try action(#"{"type":"openUrl","url":"http://example.com"}"#), ActionContext(route: home))
        XCTAssertEqual(rt.toast, L10n.tr("openUrl: url は https か line/slack/discord/tel/mailto/maps のみ"))
    }

    func testChildFormBeforeParentIsSavedAsksToSaveFirst() async throws {
        let rt = try runtime()
        await rt.run(try action(#"{"type":"navigate","screen":"expense_form","prefillRef":"parent"}"#),
                     ActionContext(route: home))
        XCTAssertEqual(rt.toast, L10n.tr("先に保存してください"))
        XCTAssertEqual(rt.paths[Route.tabStack("home")] ?? [], [], "画面は開かない")
    }

    func testReadOnlyAndSaveFailedMessages() throws {
        let rt = try runtime()
        rt.notifyReadOnly(entity: "expense")
        XCTAssertEqual(rt.toast, RecordActionSteps.readOnlyMessage)
        rt.notifySaveFailed(entity: "expense")
        XCTAssertEqual(rt.toast, RecordActionSteps.saveFailedMessage)
    }

    // MARK: - 同期の知らせ

    func testRefusedWriteShowsSaveFailedToast() async throws {
        let rt = try runtime()
        rt.recordSync.onEvent?(.rejected(appId: rt.appId, entity: "expense", message: "bad"))
        await eventually("断られた書き込みは一言で知らせる") { rt.toast == RecordActionSteps.saveFailedMessage }
    }

    func testForbiddenShowsSaveFailedToast() async throws {
        let rt = try runtime()
        rt.recordSync.onEvent?(.forbidden(appId: rt.appId, entity: "expense"))
        await eventually("作った人以外の変更も一言で知らせる") { rt.toast == RecordActionSteps.saveFailedMessage }
    }

    func testSentAndDrainedAreQuiet() async throws {
        let rt = try runtime()
        rt.recordSync.onEvent?(.sent(appId: rt.appId, entity: "expense"))
        rt.recordSync.onEvent?(.drained)
        rt.recordSync.onEvent?(.paused(pending: 1))
        rt.recordSync.onEvent?(.conflict(appId: rt.appId, entity: "expense", message: "409"))
        try await Task.sleep(for: .milliseconds(200))
        XCTAssertNil(rt.toast, "送れた・空になった・止まった・競合は知らせない")
    }

    func testSavedRecordShowsInList() async throws {
        let rt = try runtime()
        let ok = await rt.createRecord(entity: "expense", data: ["amount": .number(500), "memo": .string("コンビニ")])
        XCTAssertTrue(ok)
        await eventually("作った記録が一覧に出る") { rt.records(for: "expense").count == 1 }
        XCTAssertNil(rt.toast)
    }
}
