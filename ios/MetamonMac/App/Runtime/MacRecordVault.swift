import Foundation
import MetamonServices

/// 作ったアプリの記録を置く場所(#1284)。
/// - サーバーにあるアプリ(本物の AI で作った・`MacServices.appConfig` がある): 記録はサーバーの appconfig に読み書きする
///   (iOS と同じ `RecordSyncEngine` + `HTTPAppConfigClient`)。ローカル DB と送信待ちの列は live/ に置く(圏外でも書ける)
/// - それ以外(モック・appconfig の URL 無し・端末で複製したアプリ): 端末だけ。ローカル DB と送信待ちの列(local/)・
///   「サーバー」の行(server/・#1517 の `LocalAppConfigClient`)
/// Application Support/metamon-mac-records/<appId>/ の下に置く。同梱の例は今まで通り一時フォルダ(`MacAppRuntime.mockRecordSync`)。
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

    /// `server` があればサーバーの記録(iOS と同じく到達性は `NWPathMonitor`)、nil なら端末だけ
    @MainActor
    static func recordSync(appId: String, server: (any AppConfigClient)? = nil, root: URL = root) -> RecordSyncEngine {
        let dir = directory(appId: appId, root: root)
        if let server {
            return RecordSyncEngine(client: server, directory: dir.appendingPathComponent("live", isDirectory: true))
        }
        let client = LocalAppConfigClient(directory: dir.appendingPathComponent("server", isDirectory: true))
        return RecordSyncEngine(client: client, connectivity: ManualConnectivity(online: true),
                                directory: dir.appendingPathComponent("local", isDirectory: true))
    }

    /// アプリを消したら記録も消す(iOS の `recordSync.purge` と同じ)。端末の動画・録音も同じフォルダの中にある。
    /// サーバーの分はアプリごと消す(`MacRootView.deleteApp` が appconfig の deleteApp を呼ぶ)
    static func purge(appId: String, root: URL = root) {
        try? FileManager.default.removeItem(at: directory(appId: appId, root: root))
    }
}
