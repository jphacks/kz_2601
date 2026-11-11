import XCTest
import MetamonDSL
@testable import MetamonRenderer
import MetamonServices
@testable import Metamon

/// Mac の `export`(CSV / PDF を保存パネルで保存)とライブアクティビティの一言(#1143)
@MainActor
final class MacExportTests: XCTestCase {
    @MainActor private final class Box { var url: URL? }
    private let home = Route(screen: "home", stack: Route.tabStack("home"))
    private var original: (@MainActor (ExportFile) async -> URL?)!
    private var outDir: URL!

    override func setUp() async throws {
        original = MacExportSaver.chooseDestination
        outDir = FileManager.default.temporaryDirectory.appendingPathComponent("mac-export-test-\(UUID().uuidString)", isDirectory: true)
        try FileManager.default.createDirectory(at: outDir, withIntermediateDirectories: true)
    }

    override func tearDown() async throws {
        MacExportSaver.chooseDestination = original
        try? FileManager.default.removeItem(at: outDir)
    }

    private func runtime() throws -> MacAppRuntime {
        let url = try XCTUnwrap(Bundle(for: MacAppRuntime.self).url(forResource: "kakeibo", withExtension: "json"))
        return MacAppRuntime(appId: "test-mac-\(UUID().uuidString.lowercased())", dsl: try AppDSL.load(contentsOf: url),
                             ai: MockAIClient(delay: .zero), isAIConsentGranted: { false })
    }

    private func action(_ json: String) throws -> Action {
        try JSONDecoder().decode(Action.self, from: Data(json.utf8))
    }

    func testExportCSVSavesToChosenPlaceAndRemovesTemporaryCopy() async throws {
        let rt = try runtime()
        let dest = outDir.appendingPathComponent("out.csv")
        let offered = Box()
        MacExportSaver.chooseDestination = { file in
            offered.url = file.url
            return dest
        }
        await rt.run(try action(#"{"type":"export","entity":"expense","format":"csv"}"#), ActionContext(route: home))
        XCTAssertEqual(offered.url?.pathExtension, "csv")
        XCTAssertTrue(FileManager.default.fileExists(atPath: dest.path))
        XCTAssertEqual(rt.toast, L10n.tr("書き出しました"))
        XCTAssertFalse(FileManager.default.fileExists(atPath: ExportFiles.directory.path), "記録の写しを一時フォルダに残さない")
    }

    func testCancelDoesNotSave() async throws {
        let rt = try runtime()
        MacExportSaver.chooseDestination = { _ in nil }
        await rt.run(try action(#"{"type":"export","entity":"expense","format":"pdf"}"#), ActionContext(route: home))
        XCTAssertNil(rt.toast)
        XCTAssertFalse(FileManager.default.fileExists(atPath: ExportFiles.directory.path))
    }

    func testUnknownEntityShowsToast() async throws {
        let rt = try runtime()
        MacExportSaver.chooseDestination = { _ in XCTFail("パネルを出さない"); return nil }
        await rt.run(try action(#"{"type":"export","entity":"nope","format":"csv"}"#), ActionContext(route: home))
        XCTAssertEqual(rt.toast, L10n.tr("書き出す記録が見つかりません"))
    }

    func testSaveReplacesExistingFile() throws {
        let src = outDir.appendingPathComponent("src.csv")
        try Data("new".utf8).write(to: src)
        let dest = outDir.appendingPathComponent("dest.csv")
        try Data("old".utf8).write(to: dest)
        try MacExportSaver.save(ExportFile(url: src), to: dest)
        XCTAssertEqual(try String(contentsOf: dest, encoding: .utf8), "new")
    }

    func testLiveActivityExplainsInsteadOfUnsupported() async throws {
        let rt = try runtime()
        await rt.run(try action(#"{"type":"startActivity"}"#), ActionContext(route: home))
        XCTAssertEqual(rt.toast, L10n.tr("ライブアクティビティは iPhone だけで表示されます"))
        rt.toast = nil
        await rt.run(try action(#"{"type":"endActivity"}"#), ActionContext(route: home))
        XCTAssertNil(rt.toast)
    }
}
