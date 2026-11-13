import Foundation
#if os(iOS)
import SwiftUI
import UIKit
import UniformTypeIdentifiers
#endif

/// 一度選んだフォルダ(iCloud Drive など)を覚えて、次から選ばずに書き出し・読み込みを続ける口
/// (#2221・親 #1808)。部品・アクションはまだ無い(決定待ち)。
/// `PhotoLibrarySaver` と同じ形: protocol + 実体(`LiveFolderBookmarkBridge`)+ 偽物(`FakeFolderBookmarkBridge`)。
///
/// 肝は 4 つ:
/// - **appId がキー**: 保存先は `metamon.folderBookmark.<appId>`。アプリごとに別のフォルダを覚える。
/// - **古い bookmark(stale)**: フォルダを移した・OS を上げた後は `bookmarkDataIsStale` が立つ。
///   解決できた URL から作り直して保存し直す(次からは古くない)。作り直せなくても今回の書き出しは通す。
/// - **消えたフォルダ**: 解決できてもフォルダが無いことがある。`folderMissing` を投げる。
///   覚えた物は消さない(iCloud が一時的に降りている時に選び直させないため)。
/// - **アクセスを必ず閉じる**: `startAccessingSecurityScopedResource` は必ず `defer` で対に閉じる。
///
/// 書き出した中身(記録の写し)はログ・クラッシュ報告・サーバーに出さない。
public struct FolderBookmark: Sendable {
    public static let shared = FolderBookmark()

    let store: any FolderBookmarkStoring
    let bridge: any FolderBookmarkBridging

    public init(store: any FolderBookmarkStoring = FolderBookmarkDefaults(),
                bridge: any FolderBookmarkBridging = LiveFolderBookmarkBridge()) {
        self.store = store
        self.bridge = bridge
    }

    /// このアプリのフォルダを覚えているか(覚えていれば 2 回目は選ばせない)。
    public func isRemembered(appId: String) -> Bool { store.data(appId: appId) != nil }

    /// 選んだフォルダを覚える(`FolderPicker` の直後)。すでに覚えていれば置き換える。
    @discardableResult
    public func remember(_ folder: URL, appId: String) throws -> Data {
        let scoped = bridge.startAccess(folder)
        defer { if scoped { bridge.stopAccess(folder) } }
        let data = try bridge.bookmarkData(for: folder)
        store.save(data, appId: appId)
        return data
    }

    /// 覚えた物を忘れる(アプリを消した時・選び直す時)。
    public func forget(appId: String) { store.remove(appId: appId) }

    /// 覚えたフォルダの URL。古ければ作り直して保存し直す。
    /// 選んでいなければ `notChosen`、解決できなければ `unreadable`、フォルダが無ければ `folderMissing`。
    public func folder(appId: String) throws -> URL {
        guard let data = store.data(appId: appId) else { throw FolderBookmarkError.notChosen }
        let resolved: FolderBookmarkResolution
        do {
            resolved = try bridge.resolve(data)
        } catch {
            throw FolderBookmarkError.unreadable
        }
        let url = resolved.url
        // フォルダがあるかの確認も security-scoped の中で見る(開かないと見えないことがある)
        let scoped = bridge.startAccess(url)
        defer { if scoped { bridge.stopAccess(url) } }
        guard bridge.isDirectory(url) else { throw FolderBookmarkError.folderMissing }
        if resolved.isStale, let fresh = try? bridge.bookmarkData(for: url) {
            store.save(fresh, appId: appId)
        }
        return url
    }

    /// 覚えたフォルダを security-scoped のアクセスで包んで渡す。書き出し(CSV / JSON)と読み込みはここを通す。
    /// 閉じるのは `defer` なので、`body` が throw しても投げ出しても必ず閉じる。
    public func withFolder<T>(appId: String, _ body: (URL) throws -> T) throws -> T {
        let folder = try folder(appId: appId)
        guard bridge.startAccess(folder) else { throw FolderBookmarkError.accessDenied }
        defer { bridge.stopAccess(folder) }
        return try body(folder)
    }

    /// 覚えたフォルダにファイルを書く(同じ名前があれば置き換える)。書いた先を返す。
    @discardableResult
    public func write(_ data: Data, named name: String, appId: String,
                      fileManager: FileManager = .default) throws -> URL {
        try withFolder(appId: appId) { folder in
            let destination = folder.appending(path: Self.safeFileName(name))
            if fileManager.fileExists(atPath: destination.path) { try fileManager.removeItem(at: destination) }
            try data.write(to: destination, options: .atomic)
            return destination
        }
    }

    /// 覚えたフォルダのファイルを読む(読み込みも同じ口を通す)。
    public func read(named name: String, appId: String) throws -> Data {
        try withFolder(appId: appId) { folder in
            try Data(contentsOf: folder.appending(path: Self.safeFileName(name)))
        }
    }

    /// 一時フォルダに作った書き出し(`ExportFile`)を覚えたフォルダに写す。写した先を返す。
    @discardableResult
    public func copy(_ source: URL, appId: String, as name: String? = nil,
                     fileManager: FileManager = .default) throws -> URL {
        try withFolder(appId: appId) { folder in
            let destination = folder.appending(path: Self.safeFileName(name ?? source.lastPathComponent))
            if fileManager.fileExists(atPath: destination.path) { try fileManager.removeItem(at: destination) }
            try fileManager.copyItem(at: source, to: destination)
            return destination
        }
    }

    /// 覚えたフォルダの外に出さない名前にする(`../` やパスを落として最後の 1 つだけ使う)。
    static func safeFileName(_ name: String) -> String {
        let last = (name as NSString).lastPathComponent
        let trimmed = last.trimmingCharacters(in: .whitespacesAndNewlines)
        return trimmed.isEmpty || trimmed == ".." || trimmed == "." ? "export" : trimmed
    }
}

/// 解決した結果。`isStale` が立っていたら作り直す。
public struct FolderBookmarkResolution: Sendable, Equatable {
    public var url: URL
    public var isStale: Bool

    public init(url: URL, isStale: Bool) {
        self.url = url
        self.isStale = isStale
    }
}

public enum FolderBookmarkError: Error, Equatable {
    /// まだフォルダを選んでいない(1 回目)
    case notChosen
    /// bookmark を解決できない(壊れている)
    case unreadable
    /// 解決できたがフォルダが無い(消された・外に出された)
    case folderMissing
    /// security-scoped のアクセスを開けなかった(許可が切れた)
    case accessDenied
}

// MARK: - 保存先

/// 覚えた bookmark の保存先。テストでは `InMemoryFolderBookmarkStore` に差し替える。
public protocol FolderBookmarkStoring: Sendable {
    func data(appId: String) -> Data?
    func save(_ data: Data, appId: String)
    func remove(appId: String)
}

/// UserDefaults に置く実体。キーは `metamon.folderBookmark.<appId>`(アプリごとに別のフォルダ)。
public struct FolderBookmarkDefaults: FolderBookmarkStoring, @unchecked Sendable {
    static let prefix = "metamon.folderBookmark."

    let defaults: UserDefaults

    public init(defaults: UserDefaults = .standard) { self.defaults = defaults }

    /// アプリごとに一意なキー。
    public static func key(appId: String) -> String { prefix + appId }

    public func data(appId: String) -> Data? { defaults.data(forKey: Self.key(appId: appId)) }

    public func save(_ data: Data, appId: String) { defaults.set(data, forKey: Self.key(appId: appId)) }

    public func remove(appId: String) { defaults.removeObject(forKey: Self.key(appId: appId)) }
}

/// テスト・プレビュー用の保存先。
public final class InMemoryFolderBookmarkStore: FolderBookmarkStoring, @unchecked Sendable {
    public private(set) var saved: [String: Data] = [:]
    /// 保存した回数(作り直しが 1 回だけ起きることを見る)
    public private(set) var saveCount = 0

    public init(saved: [String: Data] = [:]) { self.saved = saved }

    public func data(appId: String) -> Data? { saved[appId] }

    public func save(_ data: Data, appId: String) {
        saved[appId] = data
        saveCount += 1
    }

    public func remove(appId: String) { saved[appId] = nil }
}

// MARK: - bookmark の作り直し・解決

/// `bookmarkData` と security-scoped のアクセス。テストでは `FakeFolderBookmarkBridge` に差し替える。
public protocol FolderBookmarkBridging: Sendable {
    /// フォルダの URL から bookmark を作る(覚える時・古くなった時の作り直し)。
    func bookmarkData(for folder: URL) throws -> Data
    /// bookmark を URL に戻す。古ければ `isStale` が立つ。
    func resolve(_ data: Data) throws -> FolderBookmarkResolution
    /// 今もフォルダとしてあるか(消えたフォルダの検出)。
    func isDirectory(_ url: URL) -> Bool
    /// security-scoped のアクセスを開く。false なら開けなかった(`stopAccess` を呼んではいけない)。
    func startAccess(_ url: URL) -> Bool
    /// 開いたアクセスを閉じる(`startAccess` が true の時だけ呼ぶ)。
    func stopAccess(_ url: URL)
}

/// 本物の `bookmarkData`。Mac は `.withSecurityScope` が要る(iOS のフォルダ選択の URL は既に scoped)。
public struct LiveFolderBookmarkBridge: FolderBookmarkBridging {
    public init() {}

    public func bookmarkData(for folder: URL) throws -> Data {
        #if os(macOS)
        try folder.bookmarkData(options: [.withSecurityScope], includingResourceValuesForKeys: nil, relativeTo: nil)
        #else
        try folder.bookmarkData(options: [], includingResourceValuesForKeys: nil, relativeTo: nil)
        #endif
    }

    public func resolve(_ data: Data) throws -> FolderBookmarkResolution {
        var stale = false
        #if os(macOS)
        let url = try URL(resolvingBookmarkData: data, options: [.withSecurityScope],
                          relativeTo: nil, bookmarkDataIsStale: &stale)
        #else
        let url = try URL(resolvingBookmarkData: data, options: [], relativeTo: nil, bookmarkDataIsStale: &stale)
        #endif
        return FolderBookmarkResolution(url: url, isStale: stale)
    }

    /// `resourceValues` は URL 1 つぶんの結果を覚えてしまい、消した後も「ある」と答える。
    /// 消えたフォルダを見分けたいので `FileManager` で毎回見る。
    public func isDirectory(_ url: URL) -> Bool {
        var isDirectory: ObjCBool = false
        let exists = FileManager.default.fileExists(atPath: url.path(percentEncoded: false),
                                                    isDirectory: &isDirectory)
        return exists && isDirectory.boolValue
    }

    public func startAccess(_ url: URL) -> Bool { url.startAccessingSecurityScopedResource() }

    public func stopAccess(_ url: URL) { url.stopAccessingSecurityScopedResource() }
}

/// テスト・プレビュー用。解決の結果・古さ・フォルダの有無・アクセスの可否を外から決められる。
/// 開いた数と閉じた数(`openAccessCount`)が合うかで、閉じ忘れ(リソースリーク)を見る。
public final class FakeFolderBookmarkBridge: FolderBookmarkBridging, @unchecked Sendable {
    public enum Failure: Error, Equatable {
        /// `bookmarkData` が作れなかった
        case cannotMake
        /// bookmark が壊れていて解決できない
        case cannotResolve
    }

    /// 解決して返す URL(nil なら `resolve` は `cannotResolve`)
    public var resolvedURL: URL?
    /// 解決した bookmark が古いか(作り直しが起きる)
    public var isStale = false
    /// 今あるフォルダ(ここに無い URL は「消えたフォルダ」)
    public var existingFolders: Set<URL>
    /// security-scoped のアクセスを開けるか
    public var canAccess = true
    public var makeError: Failure?
    public var resolveError: Failure?

    public private(set) var makeCount = 0
    public private(set) var resolveCount = 0
    public private(set) var startCount = 0
    public private(set) var stopCount = 0
    public private(set) var lastResolvedData: Data?

    /// 開いたまま閉じていない数。0 でなければ閉じ忘れ。
    public var openAccessCount: Int { startCount - stopCount }

    public init(resolvedURL: URL? = nil, isStale: Bool = false, existingFolders: Set<URL> = []) {
        self.resolvedURL = resolvedURL
        self.isStale = isStale
        self.existingFolders = existingFolders.isEmpty ? Set([resolvedURL].compactMap { $0 }) : existingFolders
    }

    public func bookmarkData(for folder: URL) throws -> Data {
        if let makeError { throw makeError }
        makeCount += 1
        return Data("bookmark#\(makeCount)@\(folder.path)".utf8)
    }

    public func resolve(_ data: Data) throws -> FolderBookmarkResolution {
        resolveCount += 1
        lastResolvedData = data
        if let resolveError { throw resolveError }
        guard let resolvedURL else { throw Failure.cannotResolve }
        return FolderBookmarkResolution(url: resolvedURL, isStale: isStale)
    }

    public func isDirectory(_ url: URL) -> Bool { existingFolders.contains(url) }

    public func startAccess(_ url: URL) -> Bool {
        guard canAccess else { return false }
        startCount += 1
        return true
    }

    public func stopAccess(_ url: URL) { stopCount += 1 }
}

// MARK: - フォルダを選ぶ画面

#if os(iOS)
/// フォルダを選ぶ画面(`UIDocumentPickerViewController`)。選んだ URL は `FolderBookmark.remember` で覚える。
/// `asCopy: false`(写しを作らず元の場所を指す)でないとフォルダは選べない。
public struct FolderPicker: UIViewControllerRepresentable {
    let onPick: (URL) -> Void
    let onCancel: () -> Void

    public init(onPick: @escaping (URL) -> Void, onCancel: @escaping () -> Void = {}) {
        self.onPick = onPick
        self.onCancel = onCancel
    }

    public func makeUIViewController(context: Context) -> UIDocumentPickerViewController {
        let picker = UIDocumentPickerViewController(forOpeningContentTypes: [.folder], asCopy: false)
        picker.allowsMultipleSelection = false
        picker.delegate = context.coordinator
        return picker
    }

    public func updateUIViewController(_ controller: UIDocumentPickerViewController, context: Context) {}

    public func makeCoordinator() -> Coordinator { Coordinator(onPick: onPick, onCancel: onCancel) }

    public final class Coordinator: NSObject, UIDocumentPickerDelegate {
        let onPick: (URL) -> Void
        let onCancel: () -> Void

        init(onPick: @escaping (URL) -> Void, onCancel: @escaping () -> Void) {
            self.onPick = onPick
            self.onCancel = onCancel
        }

        public func documentPicker(_ controller: UIDocumentPickerViewController, didPickDocumentsAt urls: [URL]) {
            guard let url = urls.first else { return onCancel() }
            onPick(url)
        }

        public func documentPickerWasCancelled(_ controller: UIDocumentPickerViewController) { onCancel() }
    }
}
#endif
