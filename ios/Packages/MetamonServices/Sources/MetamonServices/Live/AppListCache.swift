import Foundation

/// 最後に取得した自分のアプリ一覧(`GET /v1/apps`)を `DocumentCache` と同じ場所(`Application Support/metamon-dsl/apps.json`)に保存する。
/// オフライン時の変身一覧に使う(正典はサーバー)。
public final class AppListCache: @unchecked Sendable {
    public static let shared = AppListCache()

    private let fileURL: URL
    private let lock = NSLock()

    public init(directory: URL? = nil) {
        let base = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask).first
            ?? FileManager.default.temporaryDirectory
        let dir = directory ?? base.appendingPathComponent("metamon-dsl", isDirectory: true)
        self.fileURL = dir.appendingPathComponent("apps.json")
    }

    public func store(_ apps: [AppSummary]) {
        lock.withLock {
            do {
                try FileManager.default.createDirectory(at: fileURL.deletingLastPathComponent(), withIntermediateDirectories: true)
                try JSONEncoder().encode(apps).write(to: fileURL, options: .atomic)
            } catch {
                print("[Metamon] AppListCache store failed: \(error)")
            }
        }
    }

    public func load() -> [AppSummary] {
        lock.withLock {
            guard let data = try? Data(contentsOf: fileURL) else { return [] }
            return (try? JSONDecoder().decode([AppSummary].self, from: data)) ?? []
        }
    }

    public func clear() {
        lock.withLock { try? FileManager.default.removeItem(at: fileURL) }
    }
}
