import Foundation
import MetamonDSL

/// 最後に取得した DSL 封筒(`{appId, version, dsl}` の生 JSON)を appId ごとにファイル保存する。
/// 用途はオフライン起動時の復元と即時描画のみ(正典はサーバー)。レコードは保存しない。
/// `AppDocument` は Decodable のみなので、デコード前のバイト列をそのまま置く。
public final class DocumentCache: @unchecked Sendable {
    public static let shared = DocumentCache()

    private let directory: URL
    private let lock = NSLock()

    public init(directory: URL? = nil) {
        let base = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask).first
            ?? FileManager.default.temporaryDirectory
        self.directory = directory ?? base.appendingPathComponent("metamon-dsl", isDirectory: true)
    }

    public func store(appId: String, envelope: Data) {
        lock.withLock {
            do {
                try FileManager.default.createDirectory(at: directory, withIntermediateDirectories: true)
                try envelope.write(to: url(appId), options: .atomic)
            } catch {
                print("[Metamon] DocumentCache store failed: \(error)")
            }
        }
    }

    public func loadData(appId: String) -> Data? {
        lock.withLock { try? Data(contentsOf: url(appId)) }
    }

    public func load(appId: String) -> AppDocument? {
        guard let data = loadData(appId: appId) else { return nil }
        return try? AppDocument.decode(data)
    }

    public func remove(appId: String) {
        lock.withLock { try? FileManager.default.removeItem(at: url(appId)) }
    }

    /// アカウント削除用(#119): 全 appId の DSL と一覧キャッシュ(同じフォルダの apps.json)を消す。
    public func removeAll() {
        lock.withLock { try? FileManager.default.removeItem(at: directory) }
    }

    private func url(_ appId: String) -> URL {
        let safe = appId.map { $0.isLetter || $0.isNumber || $0 == "-" || $0 == "_" ? $0 : "_" }
        return directory.appendingPathComponent("\(String(safe)).json")
    }
}
