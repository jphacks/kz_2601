import Foundation

/// 最後に取得した「つないだサービス」の一覧(`GET /v1/me/connections`)を `AppListCache` と同じ場所
/// (`Application Support/metamon-dsl/connections.json`)に保存する。読めない時(オフライン)の表示に使う(正典はサーバー)。
public final class ExtConnectionListCache: @unchecked Sendable {
    public static let shared = ExtConnectionListCache()

    private let fileURL: URL
    private let lock = NSLock()

    public init(directory: URL? = nil) {
        let base = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask).first
            ?? FileManager.default.temporaryDirectory
        let dir = directory ?? base.appendingPathComponent("metamon-dsl", isDirectory: true)
        self.fileURL = dir.appendingPathComponent("connections.json")
    }

    public func store(_ list: ExtConnectionList) {
        lock.withLock {
            do {
                try FileManager.default.createDirectory(at: fileURL.deletingLastPathComponent(), withIntermediateDirectories: true)
                try JSONEncoder().encode(list).write(to: fileURL, options: .atomic)
            } catch {
                print("[Metamon] ExtConnectionListCache store failed: \(error)")
            }
        }
    }

    /// 保存した一覧。無い・読めない時は nil
    public func load() -> ExtConnectionList? {
        lock.withLock {
            guard let data = try? Data(contentsOf: fileURL) else { return nil }
            return try? JSONDecoder().decode(ExtConnectionList.self, from: data)
        }
    }

    public func clear() {
        lock.withLock { try? FileManager.default.removeItem(at: fileURL) }
    }
}
