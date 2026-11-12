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

/// `DocumentCache` に置く封筒(`{appId, version, dsl}`)を作る・直す。`AppDocument` は書き出せないので生 JSON のまま扱う。
public enum DocumentEnvelope {
    /// DSL 本文の JSON(`dsl` の中身)を封筒に包む。JSON でなければ nil
    public static func make(appId: String, version: Int, dsl: Data) -> Data? {
        guard let body = try? JSONSerialization.jsonObject(with: dsl) else { return nil }
        return try? JSONSerialization.data(withJSONObject: ["appId": appId, "version": version, "dsl": body])
    }

    /// 封筒の `dsl.app.name` を差し替える(名前の変更・#969)。読めなければ nil
    public static func renamed(_ envelope: Data, to name: String) -> Data? {
        guard var root = try? JSONSerialization.jsonObject(with: envelope) as? [String: Any],
              var dsl = root["dsl"] as? [String: Any], var app = dsl["app"] as? [String: Any] else { return nil }
        app["name"] = name
        dsl["app"] = app
        root["dsl"] = dsl
        return try? JSONSerialization.data(withJSONObject: root)
    }

    /// 複製(Mac の端末だけのアプリ・#1285): 新しい appId・版 1・名前で包み直す。画面の定義だけを写す。読めなければ nil
    public static func duplicated(_ envelope: Data, appId: String, name: String) -> Data? {
        guard let renamed = renamed(envelope, to: name),
              var root = try? JSONSerialization.jsonObject(with: renamed) as? [String: Any] else { return nil }
        root["appId"] = appId
        root["version"] = 1
        return try? JSONSerialization.data(withJSONObject: root)
    }
}
