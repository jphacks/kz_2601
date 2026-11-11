import Foundation
import MetamonServices

/// 作ったアプリの記録を端末に残す場所(#1284)。サーバー(#930)が入るまでは端末だけ。
/// Application Support/metamon-mac-records/<appId>/ に、ローカル DB と送信待ちの列(local/)・「サーバー」の行(server/)を置く。
/// 同梱の例は今まで通り一時フォルダ(`MacAppRuntime.mockRecordSync`)。
enum MacRecordVault {
    static var root: URL {
        let base = FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask).first
            ?? FileManager.default.temporaryDirectory
        return base.appendingPathComponent("metamon-mac-records", isDirectory: true)
    }

    static func directory(appId: String, root: URL = root) -> URL {
        let safe = String(appId.map { $0.isLetter || $0.isNumber || $0 == "-" || $0 == "_" ? $0 : "_" })
        return root.appendingPathComponent(safe, isDirectory: true)
    }

    @MainActor
    static func recordSync(appId: String, root: URL = root) -> RecordSyncEngine {
        let dir = directory(appId: appId, root: root)
        let client = LocalAppConfigClient(directory: dir.appendingPathComponent("server", isDirectory: true))
        return RecordSyncEngine(client: client, connectivity: ManualConnectivity(online: true),
                                directory: dir.appendingPathComponent("local", isDirectory: true))
    }

    /// アプリを消したら記録も消す(iOS の `recordSync.purge` と同じ)。端末の動画・録音も同じフォルダの中にある
    static func purge(appId: String, root: URL = root) {
        try? FileManager.default.removeItem(at: directory(appId: appId, root: root))
    }
}
