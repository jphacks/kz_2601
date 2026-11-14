import XCTest
import AppKit
import MetamonDSL
@testable import MetamonRenderer
import MetamonServices
@testable import Metamon

/// export の写しの片付け(#946・#953): 書き出しごとのフォルダを、保存・共有が終わった時にそのフォルダだけ消す
@MainActor
final class MacExportCleanupTests: XCTestCase {
    @MainActor private final class Box { var urls: [URL] = [] }
    private let home = Route(screen: "home", stack: Route.tabStack("home"))
    private var originalOutput: (@MainActor () async -> MacExportOutput?)!
    private var originalShare: (@MainActor (URL) -> Bool)!
    private var originalDestination: (@MainActor (ExportFile) async -> URL?)!
    private var outDir: URL!

    override func setUp() async throws {
        ExportFiles.removeAll()
        originalOutput = MacExportSaver.chooseOutput
        originalShare = MacExportSaver.share
        originalDestination = MacExportSaver.chooseDestination
        outDir = FileManager.default.temporaryDirectory.appendingPathComponent("mac-export-cleanup-\(UUID().uuidString)", isDirectory: true)
        try FileManager.default.createDirectory(at: outDir, withIntermediateDirectories: true)
    }

    override func tearDown() async throws {
        MacExportSaver.chooseOutput = originalOutput
        MacExportSaver.share = originalShare
        MacExportSaver.chooseDestination = originalDestination
        ExportFiles.removeAll()
        try? FileManager.default.removeItem(at: outDir)
    }

    private func runtime() throws -> MacAppRuntime {
        let url = try XCTUnwrap(Bundle(for: MacAppRuntime.self).url(forResource: "kakeibo", withExtension: "json"))
        return MacAppRuntime(appId: "test-mac-\(UUID().uuidString.lowercased())", dsl: try AppDSL.load(contentsOf: url),
                             ai: MockAIClient(delay: .zero), isAIConsentGranted: { false })
    }

    private func csv() throws -> Action {
        try JSONDecoder().decode(Action.self, from: Data(#"{"type":"export","entity":"expense","format":"csv"}"#.utf8))
    }

    /// 行のドラッグ(`ListRowCopyText.csvDragItem`)と同じ形の写し
    private func otherCopy() throws -> URL {
        let dir = ExportFiles.directory.appending(path: UUID().uuidString, directoryHint: .isDirectory)
        try FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        let url = dir.appending(path: "drag.csv")
        try Data("a".utf8).write(to: url)
        return url
    }

    private func exists(_ url: URL) -> Bool { FileManager.default.fileExists(atPath: url.path) }

    func testEachShareKeepsItsOwnFolderAndOtherWindowsCopies() async throws {
        let rt = try runtime()
        let shared = Box()
        MacExportSaver.chooseOutput = { .share }
        MacExportSaver.share = { shared.urls.append($0); return true }
        await rt.run(try csv(), ActionContext(route: home))
        await rt.run(try csv(), ActionContext(route: home))
        XCTAssertEqual(shared.urls.count, 2)
        XCTAssertNotEqual(shared.urls.first?.deletingLastPathComponent(), shared.urls.last?.deletingLastPathComponent(),
                          "書き出しごとのフォルダ")
        XCTAssertTrue(shared.urls.allSatisfy(exists), "別のウインドウで共有中の写しを次の書き出しで消さない")
    }

    func testSaveRemovesOnlyItsOwnFolder() async throws {
        let drag = try otherCopy()
        let rt = try runtime()
        let offered = Box()
        let dest = outDir.appendingPathComponent("out.csv")
        MacExportSaver.chooseOutput = { .save }
        MacExportSaver.chooseDestination = { file in
            offered.urls.append(file.url)
            return dest
        }
        await rt.run(try csv(), ActionContext(route: home))
        let temp = try XCTUnwrap(offered.urls.first)
        XCTAssertFalse(exists(temp.deletingLastPathComponent()), "保存した写しは消す")
        XCTAssertTrue(exists(drag), "行のドラッグの写しは残す")
        XCTAssertTrue(exists(dest))
    }

    func testRemoveIgnoresFilesOutsideExportFolder() throws {
        let outside = outDir.appendingPathComponent("keep.csv")
        try Data("a".utf8).write(to: outside)
        ExportFiles.remove(outside)
        XCTAssertTrue(exists(outside))
    }

    func testShareSessionRemovesCopyWhenCancelled() throws {
        let other = try otherCopy()
        let file = try otherCopy()
        let before = MacExportShareSession.activeCount
        let session = MacExportShareSession(url: file)
        XCTAssertEqual(MacExportShareSession.activeCount, before + 1, "終わるまで持つ")
        session.sharingServicePicker(NSSharingServicePicker(items: [file]), didChoose: nil)
        XCTAssertFalse(exists(file.deletingLastPathComponent()), "選ばずに閉じたら写しを消す")
        XCTAssertTrue(exists(other), "ほかの写しは残す")
        XCTAssertEqual(MacExportShareSession.activeCount, before, "終わったら手放す")
    }

    func testShareSessionKeepsCopyWhileServiceRunsThenRemovesWhenDone() throws {
        let file = try otherCopy()
        let session = MacExportShareSession(url: file)
        let service = NSSharingService(title: "test", image: NSImage(), alternateImage: nil) {}
        let picker = NSSharingServicePicker(items: [file])
        XCTAssertTrue(session.sharingServicePicker(picker, delegateFor: service) === session)
        session.sharingServicePicker(picker, didChoose: service)
        XCTAssertTrue(exists(file), "送り終えるまで残す")
        session.sharingService(service, didShareItems: [file])
        XCTAssertFalse(exists(file))
        session.finish() // 2 回目は何もしない
    }

    func testShareSessionRemovesCopyWhenSharingFails() throws {
        let file = try otherCopy()
        let session = MacExportShareSession(url: file)
        let service = NSSharingService(title: "test", image: NSImage(), alternateImage: nil) {}
        session.sharingService(service, didFailToShareItems: [file], error: CocoaError(.userCancelled))
        XCTAssertFalse(exists(file))
    }
}

/// 作った・消した時に設定の「使用量」を読み直す(#946)
@MainActor
final class MacAppLibraryUsageTests: XCTestCase {
    private func document(_ id: String) throws -> AppDocument {
        let url = try XCTUnwrap(Bundle(for: MacAppRuntime.self).url(forResource: "kakeibo", withExtension: "json"))
        return AppDocument(appId: id, version: 1, dsl: try AppDSL.load(contentsOf: url))
    }

    func testCreateAndDeleteInvalidateUsage() throws {
        let usage = PlanUsageStore()
        let library = MacAppLibrary(store: nil, usage: usage)
        let start = usage.revision
        library.apply(try document("mac-a"), created: true)
        XCTAssertEqual(usage.revision, start + 1, "作ったら古くする")
        library.apply(try document("mac-a"), created: false)
        XCTAssertEqual(usage.revision, start + 1, "直しただけでは数は変わらない")
        library.delete("mac-a")
        XCTAssertEqual(usage.revision, start + 2, "消したら古くする")
        library.delete("mac-missing")
        XCTAssertEqual(usage.revision, start + 2, "無いアプリは消さない")
    }
}
