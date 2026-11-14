import AppKit
import XCTest
import MetamonDSL
import MetamonServices
@testable import Metamon

/// 生成アプリごとのウインドウ(#1853): どのアプリを開くか・⌥クリックの判定・同じアプリのウインドウ同士の同期
@MainActor
final class MacAppWindowsTests: XCTestCase {
    func testWindowValueRoundTripsAsCodable() throws {
        let id = AppID(rawValue: "kakeibo")
        let data = try JSONEncoder().encode(id)
        XCTAssertEqual(try JSONDecoder().decode(AppID.self, from: data), id, "ウインドウの復元で同じアプリに戻る")
        XCTAssertNotEqual(AppID(rawValue: "a"), AppID(rawValue: "b"))
    }

    func testInitialSelectionOpensWindowApp() {
        let ids = ["a", "b", "c"]
        XCTAssertEqual(MacAppWindows.initialSelection(window: AppID(rawValue: "b"), restored: nil, appIds: ids), "b")
        XCTAssertEqual(MacAppWindows.initialSelection(window: AppID(rawValue: "b"), restored: "c", appIds: ids), "c",
                       "復元したウインドウはその中で選び直したアプリのまま")
        XCTAssertEqual(MacAppWindows.initialSelection(window: AppID(rawValue: "b"), restored: "gone", appIds: ids), "b")
        XCTAssertNil(MacAppWindows.initialSelection(window: AppID(rawValue: "server"), restored: nil, appIds: ids),
                     "まだ一覧に無いアプリは載るまで待つ")
    }

    func testFrameAutosaveNameIsPerApp() {
        XCTAssertNotEqual(MacAppWindows.frameAutosaveName(AppID(rawValue: "a")),
                          MacAppWindows.frameAutosaveName(AppID(rawValue: "b")))
        XCTAssertEqual(MacAppWindows.frameAutosaveName(AppID(rawValue: "a")), MacAppWindows.frameAutosaveName(AppID(rawValue: "a")))
    }

    func testOptionClickOpensNewWindow() {
        XCTAssertTrue(MacAppWindows.opensNewWindow(eventType: .leftMouseDown, modifiers: .option))
        XCTAssertTrue(MacAppWindows.opensNewWindow(eventType: .leftMouseUp, modifiers: .option))
        XCTAssertFalse(MacAppWindows.opensNewWindow(eventType: .leftMouseDown, modifiers: []), "ふつうのクリックは選ぶ")
        XCTAssertFalse(MacAppWindows.opensNewWindow(eventType: .leftMouseDown, modifiers: [.option, .command]))
        XCTAssertFalse(MacAppWindows.opensNewWindow(eventType: .keyDown, modifiers: .option), "⌥↑↓ のキーは今までどおり")
        XCTAssertFalse(MacAppWindows.opensNewWindow(eventType: nil, modifiers: .option))
    }

    func testWindowGroupIdDiffersFromMain() {
        XCTAssertNotEqual(MacAppWindows.windowId, MacCommands.mainWindowId)
    }

    // MARK: - 同じアプリの 2 つのウインドウ

    private var root: URL!
    private var engines: MacRecordEngines!

    override func setUp() {
        super.setUp()
        root = FileManager.default.temporaryDirectory.appendingPathComponent("mac-windows-\(UUID().uuidString)")
        engines = MacRecordEngines(root: root)
    }

    override func tearDown() {
        try? FileManager.default.removeItem(at: root)
        super.tearDown()
    }

    private func window(_ appId: String) throws -> MacAppRuntime {
        let url = try XCTUnwrap(Bundle(for: MacAppRuntime.self).url(forResource: "kakeibo", withExtension: "json"))
        return MacAppRuntime(appId: appId, dsl: try AppDSL.load(contentsOf: url), recordLease: engines.acquire(appId: appId),
                             ai: MockAIClient(delay: .zero), isAIConsentGranted: { false })
    }

    func testLocalChangeReachesOtherLeasesOnly() {
        let a = engines.acquire(appId: "x")
        let b = engines.acquire(appId: "x")
        let other = engines.acquire(appId: "y")
        var got: [String] = []
        a.onLocalChange = { got.append("a:\($0)") }
        b.onLocalChange = { got.append("b:\($0)") }
        other.onLocalChange = { got.append("y:\($0)") }
        a.announceLocalChange(entity: "expense")
        XCTAssertEqual(got, ["b:expense"], "書いた本人と別のアプリには返さない")
        b.release()
        a.announceLocalChange(entity: "expense")
        XCTAssertEqual(got, ["b:expense"], "閉じたウインドウには届かない")
        a.release()
        other.release()
    }

    func testAddInOneWindowShowsInTheOther() async throws {
        let a = try window("shared-app")
        let b = try window("shared-app")
        let entity = try XCTUnwrap(a.dsl.entities.first?.id)
        await a.load(entities: [entity], force: true)
        await b.load(entities: [entity], force: true)
        let before = b.records(for: entity).count

        let created = await a.book.create(entity: entity, data: ["memo": .string("from-a")])
        XCTAssertTrue(created)

        // b はエンジンの送信を待たずに端末の内容で読み直す
        for _ in 0..<50 where b.records(for: entity).count == before {
            try await Task.sleep(for: .milliseconds(20))
        }
        XCTAssertEqual(b.records(for: entity).count, before + 1)
    }
}
