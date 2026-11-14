import XCTest
import MetamonDSL
import MetamonServices
@testable import Metamon

/// 端末だけに置く作ったアプリの一覧(#969・Mac)。一覧・DSL・今のアプリの保存と、生成の結果の封筒。
final class LocalAppLibraryTests: XCTestCase {
    private var dir: URL!
    private var defaults: UserDefaults!
    private var suite: String!

    override func setUp() {
        dir = FileManager.default.temporaryDirectory.appendingPathComponent("local-app-library-\(UUID().uuidString)")
        suite = "LocalAppLibraryTests-\(UUID().uuidString)"
        defaults = UserDefaults(suiteName: suite)
    }

    override func tearDown() {
        try? FileManager.default.removeItem(at: dir)
        defaults.removePersistentDomain(forName: suite)
    }

    private var library: LocalAppLibrary {
        LocalAppLibrary(list: AppListCache(directory: dir), documents: DocumentCache(directory: dir), defaults: defaults)
    }

    func testMockMorphReturnsEnvelopeThatDecodesToTheSameApp() async throws {
        let result = try await MockAIClient(delay: .zero).morph(transcript: "家計簿のアプリ", appId: "mac-1", currentVersion: 0)
        let envelope = try XCTUnwrap(result.envelope)
        let decoded = try AppDocument.decode(envelope)
        XCTAssertEqual(decoded.appId, "mac-1")
        XCTAssertEqual(decoded.version, result.document.version)
        XCTAssertEqual(decoded.dsl.app.name, result.document.dsl.app.name)
    }

    func testSavedListReloadsInOrderWithRenamedName() async throws {
        let a = try await MockAIClient(delay: .zero).morph(transcript: "家計簿のアプリ", appId: "a", currentVersion: 0)
        let b = try await MockAIClient(delay: .zero).morph(transcript: "買い物リストのアプリ", appId: "b", currentVersion: 0)
        let renamed = try XCTUnwrap(DocumentEnvelope.renamed(try XCTUnwrap(a.envelope), to: "仕事の家計簿"))
        library.store(appId: "a", envelope: renamed)
        library.store(appId: "b", envelope: try XCTUnwrap(b.envelope))
        library.save(LocalAppLibrary.summaries([("b", b.document.dsl.app.name, 1), ("a", "仕事の家計簿", 1), ("gone", "消えた", 1)]))

        let entries = library.load()
        XCTAssertEqual(entries.map(\.summary.id), ["b", "a"]) // DSL の無い "gone" は外す
        XCTAssertEqual(entries.map(\.summary.sortIndex), [1, 2])
        XCTAssertEqual(entries[1].document.dsl.app.name, "仕事の家計簿")
    }

    /// 封筒の無い版を当てても一覧は進めない(古い DSL に新しい名前・版を付けない)
    func testRowStaysWithoutEnvelope() async throws {
        let a = try await MockAIClient(delay: .zero).morph(transcript: "家計簿のアプリ", appId: "a", currentVersion: 0)
        let previous = LocalAppLibrary.Row(name: "前の名前", version: 1)
        var next = a.document
        next.dsl.app.name = "新しい名前"
        next.version = 2

        XCTAssertEqual(LocalAppLibrary.row(after: next, envelope: nil, previous: previous), previous)
        XCTAssertNil(LocalAppLibrary.row(after: next, envelope: nil, previous: nil))
        XCTAssertEqual(LocalAppLibrary.row(after: nil, envelope: Data(), previous: previous), previous)
        XCTAssertEqual(LocalAppLibrary.row(after: next, envelope: Data(), previous: previous), .init(name: "新しい名前", version: 2))
    }

    func testCurrentAppIsKeptAndClearedOnRemove() {
        library.currentAppId = "a"
        XCTAssertEqual(library.currentAppId, "a")
        XCTAssertEqual(LocalAppLibrary.reopen(library.currentAppId, among: ["b", "a"]), "a")
        XCTAssertNil(LocalAppLibrary.reopen("x", among: ["b", "a"]))
        library.remove(appId: "a")
        XCTAssertNil(library.currentAppId)
    }

    /// 会話で直した同梱の例(#1283): 残して読み直せる・元に戻すと消える・作ったアプリの一覧には出ない
    func testEditedExampleIsKeptAndRemoved() async throws {
        let edited = try await MockAIClient(delay: .zero).morph(transcript: "家計簿のアプリ", appId: "expense", currentVersion: 1)
        library.currentAppId = "expense"
        library.storeExample(appId: "expense", envelope: try XCTUnwrap(edited.envelope))
        library.storeExample(appId: "expense", envelope: try XCTUnwrap(edited.envelope))

        let loaded = library.loadEditedExamples()
        XCTAssertEqual(Array(loaded.keys), ["expense"])
        XCTAssertEqual(loaded["expense"]?.document.dsl.app.name, edited.document.dsl.app.name)
        XCTAssertTrue(library.load().isEmpty)

        library.removeExample(appId: "expense")
        XCTAssertTrue(library.loadEditedExamples().isEmpty)
        XCTAssertEqual(library.currentAppId, "expense", "元に戻しても開いていた例はそのまま")
    }

    func testNeighborAfterDelete() {
        XCTAssertEqual(LocalAppLibrary.neighbor(deleting: "a", in: ["a", "b", "c"]), "b")
        XCTAssertEqual(LocalAppLibrary.neighbor(deleting: "c", in: ["a", "b", "c"]), "b")
        XCTAssertNil(LocalAppLibrary.neighbor(deleting: "a", in: ["a"]))
    }
}
