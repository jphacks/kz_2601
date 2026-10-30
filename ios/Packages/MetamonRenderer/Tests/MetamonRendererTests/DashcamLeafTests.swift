#if !APP_STORE_REVIEW
import XCTest
import SwiftUI
import MetamonDSL
@testable import MetamonRenderer

/// `dashcam`(#410): 端末内の置き場(区間の確定・保存済みへの移動・消去)・常に出す説明・描画。
/// カメラ・録画そのものは実機でしか確かめられない(シミュレータにはカメラが無い / 仮想カメラ)。
final class DashcamLeafTests: XCTestCase {
    private var base: URL!
    private var savedLanguage: AppLanguage = .system

    override func setUpWithError() throws {
        base = FileManager.default.temporaryDirectory.appendingPathComponent("dashcam-tests-\(UUID().uuidString)", isDirectory: true)
        savedLanguage = LanguagePreference.shared.setting
        LanguagePreference.shared.setting = .ja
    }

    override func tearDownWithError() throws {
        try? FileManager.default.removeItem(at: base)
        LanguagePreference.shared.setting = savedLanguage
    }

    private func write(_ url: URL, bytes: Int) throws {
        try Data(repeating: 1, count: bytes).write(to: url)
    }

    func testStoreFinalizeSaveDelete() throws {
        let store = DashcamStore(appName: "置くだけドラレコ", base: base)
        store.prepare()
        let t0 = Date(timeIntervalSince1970: 1_800_000_000)

        // 落ちて閉じられなかった録画中のファイルは prepare で消える
        let stale = store.recordingURL(start: t0.addingTimeInterval(-999))
        try write(stale, bytes: 10)
        store.prepare()
        XCTAssertFalse(FileManager.default.fileExists(atPath: stale.path))

        let rec1 = store.recordingURL(start: t0)
        try write(rec1, bytes: 1000)
        let s1 = try XCTUnwrap(store.finalize(rec1, start: t0, duration: 60, saved: false))
        let rec2 = store.recordingURL(start: t0.addingTimeInterval(60))
        try write(rec2, bytes: 2000)
        let s2 = try XCTUnwrap(store.finalize(rec2, start: t0.addingTimeInterval(60), duration: 60, saved: true))
        XCTAssertNil(store.finalize(store.recordingURL(start: t0.addingTimeInterval(120)), start: t0, duration: 1, saved: false), "ファイルが無ければ区間にしない")

        XCTAssertEqual(store.segments(), [s1, s2])
        XCTAssertEqual(store.segments().map(\.bytes), [1000, 2000])

        let moved = try XCTUnwrap(store.save(s1))
        XCTAssertTrue(moved.saved)
        XCTAssertTrue(FileManager.default.fileExists(atPath: store.url(for: moved).path))
        XCTAssertFalse(FileManager.default.fileExists(atPath: store.url(for: s1).path))

        store.delete(s2)
        XCTAssertEqual(store.segments(), [moved])

        // ループの判定にそのまま渡せる(保存済みは消さない)
        XCTAssertEqual(DashcamLoop.expired(store.segments(), now: t0.addingTimeInterval(9999), keepSeconds: 60, maxBytes: 1), [])
    }

    func testStoreIsPerApp() {
        let a = DashcamStore(appName: "A", base: base)
        let b = DashcamStore(appName: "B", base: base)
        XCTAssertNotEqual(a.root, b.root)
        XCTAssertEqual(a.root.deletingLastPathComponent(), base)
    }

    func testFootnoteAlwaysSaysScreenMustStayOn() {
        LanguagePreference.shared.setting = .en
        XCTAssertTrue(DashcamLeaf.footnote(DashcamConfig()).contains("only while this screen is showing"), "英訳がある")
        LanguagePreference.shared.setting = .ja
        let manual = DashcamLeaf.footnote(DashcamConfig())
        XCTAssertTrue(manual.contains("画面を出している間だけ"), manual)
        XCTAssertTrue(manual.contains("10"), manual)
        XCTAssertFalse(manual.contains("自動"), manual)

        let auto = DashcamLeaf.footnote(DashcamConfig(keepMinutes: 5, autoStart: ["charging", "landscape", "still"]))
        XCTAssertTrue(auto.contains("横向き・ほぼ静止・充電中"), auto)
        XCTAssertTrue(auto.contains("5"), auto)
        XCTAssertNil(DashcamLeaf.conditionText([]))
        XCTAssertEqual(DashcamLeaf.clock(3725), "1:02:05")
    }

    /// 例の DSL から部品を描ける(レジストリに登録されている・落ちない)。
    @MainActor func testRendersFromExample() throws {
        let dsl = try ExampleLoader.load("dashcam")
        let comp = try XCTUnwrap(dsl.screens.first?.components.first)
        XCTAssertEqual(comp.type, .dashcam)
        XCTAssertNotNil(ComponentRegistry.standard.builder(for: "dashcam"))

        let view = DashcamLeaf(c: comp, ctx: RenderContext(route: Route(screen: "home")))
            .frame(width: 390, height: 600)
        let renderer = ImageRenderer(content: view)
        XCTAssertNotNil(renderer.uiImage)
    }
}

/// docs/dsl/examples をテストから読む(このファイル → Tests → MetamonRenderer → Packages → ios → リポジトリ root)。
private enum ExampleLoader {
    static func load(_ name: String) throws -> AppDSL {
        var url = URL(fileURLWithPath: #filePath)
        for _ in 0..<6 { url.deleteLastPathComponent() }
        return try AppDSL.load(contentsOf: url.appending(path: "docs/dsl/examples/\(name).json"))
    }
}
#endif
