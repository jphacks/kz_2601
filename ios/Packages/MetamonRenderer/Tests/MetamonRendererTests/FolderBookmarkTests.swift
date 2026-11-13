import XCTest
@testable import MetamonRenderer

/// 決めたフォルダを覚えて書き出し続ける口(#2221)。
/// 保存 / 解決 / 古い bookmark の作り直し / 消えたフォルダ、そして
/// `startAccessingSecurityScopedResource` の開いた数と閉じた数が必ず合うことを見る。
final class FolderBookmarkTests: XCTestCase {
    private let folder = URL(fileURLWithPath: "/private/var/mobile/iCloud~Drive/Metamon", isDirectory: true)
    private let appId = "app-1"

    private func make(isStale: Bool = false, exists: Bool = true, canAccess: Bool = true)
        -> (FolderBookmark, InMemoryFolderBookmarkStore, FakeFolderBookmarkBridge) {
        let store = InMemoryFolderBookmarkStore()
        let bridge = FakeFolderBookmarkBridge(resolvedURL: folder, isStale: isStale,
                                              existingFolders: exists ? [folder] : [URL(fileURLWithPath: "/nowhere")])
        bridge.canAccess = canAccess
        return (FolderBookmark(store: store, bridge: bridge), store, bridge)
    }

    // MARK: - 保存

    func testRememberSavesBookmarkUnderAppId() throws {
        let (bookmark, store, bridge) = make()
        XCTAssertFalse(bookmark.isRemembered(appId: appId))

        try bookmark.remember(folder, appId: appId)

        XCTAssertTrue(bookmark.isRemembered(appId: appId))
        XCTAssertEqual(store.saveCount, 1)
        XCTAssertEqual(bridge.makeCount, 1)
        XCTAssertEqual(bridge.openAccessCount, 0, "覚える時に開いたアクセスも閉じる")
    }

    /// アプリごとに別のフォルダを覚える(appId がキー)。
    func testBookmarksAreSeparatePerApp() throws {
        let (bookmark, store, _) = make()
        let other = URL(fileURLWithPath: "/private/var/mobile/Other", isDirectory: true)

        try bookmark.remember(folder, appId: "app-1")
        try bookmark.remember(other, appId: "app-2")

        XCTAssertNotEqual(store.saved["app-1"], store.saved["app-2"])
        bookmark.forget(appId: "app-1")
        XCTAssertFalse(bookmark.isRemembered(appId: "app-1"))
        XCTAssertTrue(bookmark.isRemembered(appId: "app-2"), "ほかのアプリの覚えた物は消さない")
    }

    func testDefaultsStoreKeyIsScopedToAppId() {
        let defaults = UserDefaults(suiteName: "FolderBookmarkTests.\(UUID().uuidString)")!
        let store = FolderBookmarkDefaults(defaults: defaults)
        XCTAssertEqual(FolderBookmarkDefaults.key(appId: "abc"), "metamon.folderBookmark.abc")

        store.save(Data("x".utf8), appId: "abc")

        XCTAssertEqual(defaults.data(forKey: "metamon.folderBookmark.abc"), Data("x".utf8))
        XCTAssertNil(store.data(appId: "other"))
        store.remove(appId: "abc")
        XCTAssertNil(store.data(appId: "abc"))
    }

    // MARK: - 解決

    func testResolveReturnsRememberedFolderWithoutChoosingAgain() throws {
        let (bookmark, store, bridge) = make()
        try bookmark.remember(folder, appId: appId)

        XCTAssertEqual(try bookmark.folder(appId: appId), folder)
        XCTAssertEqual(bridge.resolveCount, 1)
        XCTAssertEqual(store.saveCount, 1, "古くなければ保存し直さない")
        XCTAssertEqual(bridge.openAccessCount, 0)
    }

    func testResolveThrowsWhenNothingChosen() {
        let (bookmark, _, bridge) = make()
        XCTAssertThrowsError(try bookmark.folder(appId: appId)) {
            XCTAssertEqual($0 as? FolderBookmarkError, .notChosen)
        }
        XCTAssertEqual(bridge.resolveCount, 0)
    }

    func testResolveThrowsUnreadableOnBrokenBookmark() throws {
        let (bookmark, _, bridge) = make()
        try bookmark.remember(folder, appId: appId)
        bridge.resolveError = .cannotResolve

        XCTAssertThrowsError(try bookmark.folder(appId: appId)) {
            XCTAssertEqual($0 as? FolderBookmarkError, .unreadable)
        }
        XCTAssertEqual(bridge.openAccessCount, 0, "解決できなくてもアクセスは開いたままにしない")
    }

    // MARK: - 古い bookmark の作り直し

    func testStaleBookmarkIsRecreatedAndSavedBack() throws {
        let (bookmark, store, bridge) = make()
        try bookmark.remember(folder, appId: appId)
        let first = store.saved[appId]
        bridge.isStale = true

        XCTAssertEqual(try bookmark.folder(appId: appId), folder, "古くても今回の書き出しは通す")

        XCTAssertEqual(store.saveCount, 2, "作り直して保存し直す")
        XCTAssertNotEqual(store.saved[appId], first, "新しい bookmark に入れ替わる")
        XCTAssertEqual(bridge.makeCount, 2)
        XCTAssertEqual(bridge.openAccessCount, 0)

        // 2 回目は古くない: もう作り直さない
        bridge.isStale = false
        XCTAssertEqual(try bookmark.folder(appId: appId), folder)
        XCTAssertEqual(store.saveCount, 2)
    }

    /// 作り直しが失敗しても、解決できた URL はそのまま使える(古い bookmark は残す)。
    func testStaleRecreationFailureKeepsOldBookmark() throws {
        let (bookmark, store, bridge) = make()
        try bookmark.remember(folder, appId: appId)
        let first = store.saved[appId]
        bridge.isStale = true
        bridge.makeError = .cannotMake

        XCTAssertEqual(try bookmark.folder(appId: appId), folder)
        XCTAssertEqual(store.saved[appId], first)
        XCTAssertEqual(store.saveCount, 1)
    }

    // MARK: - 消えたフォルダ

    func testMissingFolderThrowsAndKeepsTheChoice() throws {
        let (bookmark, store, bridge) = make(exists: false)
        try bookmark.remember(folder, appId: appId)

        XCTAssertThrowsError(try bookmark.folder(appId: appId)) {
            XCTAssertEqual($0 as? FolderBookmarkError, .folderMissing)
        }
        XCTAssertNotNil(store.saved[appId], "iCloud が一時的に降りている時に選び直させない")
        XCTAssertEqual(bridge.openAccessCount, 0, "消えていてもアクセスは閉じる")
    }

    func testMissingFolderStopsWriteAndRead() throws {
        let (bookmark, _, _) = make(exists: false)
        try bookmark.remember(folder, appId: appId)

        XCTAssertThrowsError(try bookmark.write(Data("a,b".utf8), named: "x.csv", appId: appId))
        XCTAssertThrowsError(try bookmark.read(named: "x.csv", appId: appId))
    }

    // MARK: - security-scoped のアクセス

    func testAccessIsAlwaysClosedEvenWhenBodyThrows() throws {
        struct Boom: Error {}
        let (bookmark, _, bridge) = make()
        try bookmark.remember(folder, appId: appId)

        XCTAssertThrowsError(try bookmark.withFolder(appId: appId) { _ -> Int in throw Boom() })

        XCTAssertGreaterThan(bridge.startCount, 0)
        XCTAssertEqual(bridge.startCount, bridge.stopCount, "開いた数と閉じた数が合う")
        XCTAssertEqual(bridge.openAccessCount, 0)
    }

    func testAccessDeniedWhenScopeCannotBeOpened() throws {
        let (bookmark, _, bridge) = make()
        try bookmark.remember(folder, appId: appId)
        bridge.canAccess = false
        let stopsBefore = bridge.stopCount

        XCTAssertThrowsError(try bookmark.withFolder(appId: appId) { $0 }) {
            XCTAssertEqual($0 as? FolderBookmarkError, .accessDenied)
        }
        XCTAssertEqual(bridge.stopCount, stopsBefore, "開けなかった時は閉じない(閉じると不正な解放になる)")
        XCTAssertEqual(bridge.openAccessCount, 0)
    }

    // MARK: - 書き出し・読み込み

    func testWriteThenReadInRememberedFolder() throws {
        let real = FileManager.default.temporaryDirectory
            .appending(path: UUID().uuidString, directoryHint: .isDirectory)
        try FileManager.default.createDirectory(at: real, withIntermediateDirectories: true)
        defer { try? FileManager.default.removeItem(at: real) }
        let store = InMemoryFolderBookmarkStore()
        let bridge = FakeFolderBookmarkBridge(resolvedURL: real, existingFolders: [real])
        let bookmark = FolderBookmark(store: store, bridge: bridge)
        try bookmark.remember(real, appId: appId)

        let written = try bookmark.write(Data("日付,値\n".utf8), named: "records.csv", appId: appId)

        XCTAssertEqual(written.lastPathComponent, "records.csv")
        XCTAssertEqual(written.deletingLastPathComponent().path, real.path)
        XCTAssertEqual(try bookmark.read(named: "records.csv", appId: appId), Data("日付,値\n".utf8))
        // 2 回目は同じ名前でも書き換えられる(選び直さない)
        try bookmark.write(Data("x".utf8), named: "records.csv", appId: appId)
        XCTAssertEqual(try bookmark.read(named: "records.csv", appId: appId), Data("x".utf8))
        XCTAssertEqual(bridge.openAccessCount, 0)
    }

    func testCopyPutsExportFileInRememberedFolder() throws {
        let real = FileManager.default.temporaryDirectory
            .appending(path: UUID().uuidString, directoryHint: .isDirectory)
        try FileManager.default.createDirectory(at: real, withIntermediateDirectories: true)
        defer { try? FileManager.default.removeItem(at: real) }
        let source = FileManager.default.temporaryDirectory.appending(path: "\(UUID().uuidString).csv")
        try Data("a,b\n".utf8).write(to: source)
        defer { try? FileManager.default.removeItem(at: source) }
        let bookmark = FolderBookmark(store: InMemoryFolderBookmarkStore(),
                                      bridge: FakeFolderBookmarkBridge(resolvedURL: real, existingFolders: [real]))
        try bookmark.remember(real, appId: appId)

        let copied = try bookmark.copy(source, appId: appId, as: "記録.csv")

        XCTAssertEqual(copied, real.appending(path: "記録.csv"))
        XCTAssertEqual(try Data(contentsOf: copied), Data("a,b\n".utf8))
        XCTAssertNoThrow(try bookmark.copy(source, appId: appId, as: "記録.csv"), "同じ名前は置き換える")
    }

    /// 覚えたフォルダの外には出さない。
    func testFileNameStaysInsideTheFolder() {
        XCTAssertEqual(FolderBookmark.safeFileName("../../etc/passwd"), "passwd")
        XCTAssertEqual(FolderBookmark.safeFileName("/tmp/a.csv"), "a.csv")
        XCTAssertEqual(FolderBookmark.safeFileName("  "), "export")
        XCTAssertEqual(FolderBookmark.safeFileName(".."), "export")
        XCTAssertEqual(FolderBookmark.safeFileName("records.csv"), "records.csv")
    }

    // MARK: - 本物の bookmarkData

    /// 本物の `bookmarkData` で作って解決し直せる(フォルダを消したら消えたと分かる)。
    func testLiveBridgeRoundTrip() throws {
        let bridge = LiveFolderBookmarkBridge()
        let dir = FileManager.default.temporaryDirectory
            .appending(path: UUID().uuidString, directoryHint: .isDirectory)
        try FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)

        let data = try bridge.bookmarkData(for: dir)
        let resolved = try bridge.resolve(data)

        XCTAssertEqual(resolved.url.resolvingSymlinksInPath().path, dir.resolvingSymlinksInPath().path)
        XCTAssertTrue(bridge.isDirectory(resolved.url))

        try FileManager.default.removeItem(at: dir)
        XCTAssertFalse(bridge.isDirectory(resolved.url), "消えたフォルダは分かる")
    }

    func testLiveBridgeRejectsGarbageBookmark() {
        XCTAssertThrowsError(try LiveFolderBookmarkBridge().resolve(Data("not a bookmark".utf8)))
    }
}
