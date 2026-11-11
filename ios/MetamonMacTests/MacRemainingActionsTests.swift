import XCTest
import AppKit
import PDFKit
import MetamonDSL
@testable import MetamonRenderer
import MetamonServices
@testable import Metamon

/// Mac の残りのアクション(#946・#953): export の共有・プリント、openVoice、follow / unfollow
@MainActor
final class MacRemainingActionsTests: XCTestCase {
    @MainActor private final class Box<T> { var value: T?; var count = 0 }
    private let home = Route(screen: "home", stack: Route.tabStack("home"))
    private var originalOutput: (@MainActor () async -> MacExportOutput?)!
    private var originalShare: (@MainActor (URL) -> Bool)!
    private var originalPrint: (@MainActor (PDFDocument) -> Bool)!
    private var originalDestination: (@MainActor (ExportFile) async -> URL?)!

    override func setUp() async throws {
        originalOutput = MacExportSaver.chooseOutput
        originalShare = MacExportSaver.share
        originalPrint = MacExportSaver.print
        originalDestination = MacExportSaver.chooseDestination
        MacExportSaver.chooseDestination = { _ in XCTFail("保存パネルは出さない"); return nil }
    }

    override func tearDown() async throws {
        MacExportSaver.chooseOutput = originalOutput
        MacExportSaver.share = originalShare
        MacExportSaver.print = originalPrint
        MacExportSaver.chooseDestination = originalDestination
        ExportFiles.removeAll()
    }

    private func runtime() throws -> MacAppRuntime {
        let url = try XCTUnwrap(Bundle(for: MacAppRuntime.self).url(forResource: "kakeibo", withExtension: "json"))
        return MacAppRuntime(appId: "test-mac-\(UUID().uuidString.lowercased())", dsl: try AppDSL.load(contentsOf: url),
                             ai: MockAIClient(delay: .zero), isAIConsentGranted: { false })
    }

    private func action(_ json: String) throws -> Action {
        try JSONDecoder().decode(Action.self, from: Data(json.utf8))
    }

    // MARK: - export: 共有・プリント

    func testAlertButtonsMapToOutputs() {
        XCTAssertEqual(MacExportSaver.output(for: .alertFirstButtonReturn), .save)
        XCTAssertEqual(MacExportSaver.output(for: .alertSecondButtonReturn), .share)
        XCTAssertEqual(MacExportSaver.output(for: .alertThirdButtonReturn), .print)
        XCTAssertNil(MacExportSaver.output(for: NSApplication.ModalResponse(rawValue: 1003)), "キャンセル")
    }

    func testShareHandsCSVFileToPickerAndKeepsItUntilNextExport() async throws {
        let rt = try runtime()
        let shared = Box<URL>()
        MacExportSaver.chooseOutput = { .share }
        MacExportSaver.share = { url in
            shared.value = url
            return true
        }
        MacExportSaver.print = { _ in XCTFail("プリントしない"); return false }
        await rt.run(try action(#"{"type":"export","entity":"expense","format":"csv"}"#), ActionContext(route: home))
        let url = try XCTUnwrap(shared.value)
        XCTAssertEqual(url.pathExtension, "csv")
        XCTAssertTrue(FileManager.default.fileExists(atPath: url.path), "共有先が読み終えるまで写しを残す")
        XCTAssertNil(rt.toast)
    }

    func testShareWithoutWindowRemovesCopyAndTellsUser() async throws {
        let rt = try runtime()
        MacExportSaver.chooseOutput = { .share }
        MacExportSaver.share = { _ in false }
        await rt.run(try action(#"{"type":"export","entity":"expense","format":"pdf"}"#), ActionContext(route: home))
        XCTAssertEqual(rt.toast, L10n.tr("書き出せませんでした"))
        XCTAssertFalse(FileManager.default.fileExists(atPath: ExportFiles.directory.path))
    }

    func testPrintSendsPDFOfTableEvenForCSVButton() async throws {
        let rt = try runtime()
        let printed = Box<PDFDocument>()
        MacExportSaver.chooseOutput = { .print }
        MacExportSaver.share = { _ in XCTFail("共有しない"); return false }
        MacExportSaver.print = { document in
            printed.value = document
            return true
        }
        await rt.run(try action(#"{"type":"export","entity":"expense","format":"csv"}"#), ActionContext(route: home))
        let document = try XCTUnwrap(printed.value)
        XCTAssertGreaterThan(document.pageCount, 0)
        XCTAssertNil(rt.toast)
        XCTAssertFalse(FileManager.default.fileExists(atPath: ExportFiles.directory.path), "プリントは一時ファイルを作らない")
    }

    func testPrintFailureShowsToast() async throws {
        let rt = try runtime()
        MacExportSaver.chooseOutput = { .print }
        MacExportSaver.print = { _ in false }
        await rt.run(try action(#"{"type":"export","entity":"expense","format":"pdf"}"#), ActionContext(route: home))
        XCTAssertEqual(rt.toast, L10n.tr("プリントできませんでした"))
    }

    func testCancelAtChoiceDoesNothing() async throws {
        let rt = try runtime()
        MacExportSaver.chooseOutput = { nil }
        MacExportSaver.share = { _ in XCTFail("共有しない"); return false }
        MacExportSaver.print = { _ in XCTFail("プリントしない"); return false }
        await rt.run(try action(#"{"type":"export","entity":"expense","format":"csv"}"#), ActionContext(route: home))
        XCTAssertNil(rt.toast)
        XCTAssertFalse(FileManager.default.fileExists(atPath: ExportFiles.directory.path))
    }

    // MARK: - openVoice

    func testOpenVoiceOpensConversation() async throws {
        let rt = try runtime()
        let opened = Box<Void>()
        rt.onOpenVoice = { opened.count += 1 }
        await rt.run(try action(#"{"type":"openVoice"}"#), ActionContext(route: home))
        XCTAssertEqual(opened.count, 1)
        XCTAssertNil(rt.toast)
    }

    func testOpenVoiceFromSheetClosesSheetFirst() async throws {
        let rt = try runtime()
        let opened = Box<Void>()
        rt.onOpenVoice = { opened.count += 1 }
        await rt.run(try action(#"{"type":"navigate","screen":"expense_form","presentation":"sheet"}"#),
                     ActionContext(route: home))
        let sheetRoute = try XCTUnwrap(rt.sheet)
        await rt.run(try action(#"{"type":"openVoice"}"#), ActionContext(route: sheetRoute))
        XCTAssertNil(rt.sheet)
        XCTAssertEqual(opened.count, 0, "シートを閉じ終えるまで開かない(シートを重ねない)")
        rt.openVoiceAfterModal() // シートの onDismiss
        XCTAssertEqual(opened.count, 1)
        rt.openVoiceAfterModal()
        XCTAssertEqual(opened.count, 1, "1 回だけ開く")
    }

    func testOpenVoiceWithoutHostSaysUnavailable() async throws {
        let rt = try runtime()
        await rt.run(try action(#"{"type":"openVoice"}"#), ActionContext(route: home))
        XCTAssertEqual(rt.toast, L10n.tr("このボタンは Mac ではまだ使えません"))
    }

    // MARK: - follow / unfollow

    func testFollowExplainsNotOnMac() async throws {
        let rt = try runtime()
        for type in ["follow", "unfollow"] {
            rt.toast = nil
            await rt.run(try action(#"{"type":"\#(type)"}"#), ActionContext(route: home))
            XCTAssertEqual(rt.toast, L10n.tr("フォローは Mac では使えません"))
        }
    }
}
