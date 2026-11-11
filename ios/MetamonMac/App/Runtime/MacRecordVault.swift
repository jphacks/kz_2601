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
        let engine = RecordSyncEngine(client: client, connectivity: ManualConnectivity(online: true),
                                      directory: dir.appendingPathComponent("local", isDirectory: true))
        live.removeAll { $0.engine == nil && $0.queue == nil }
        live.append(LiveEngine(directory: dir, engine: engine, queue: engine.queue))
        return engine
    }

    /// 作ったエンジン(ほかのウインドウ・送信の途中のものも)。消す前に止めるため弱く持つ。
    /// ランタイムを捨てた後も送信の列が送っている途中なら列だけ残る
    @MainActor private static var live: [LiveEngine] = []

    private struct LiveEngine {
        let directory: URL
        weak var engine: RecordSyncEngine?
        weak var queue: SyncQueue?
    }

    /// アプリを消したら記録も消す(iOS の `recordSync.purge` と同じ)。端末の動画・録音も同じフォルダの中にある。
    /// 先にそのアプリのエンジンを止めて送信待ちを捨て、送っている途中の 1 件が終わるのを待ってから消す
    /// (後から書かれてフォルダが作り直されないように)
    @MainActor
    static func purge(appId: String, root: URL = root, timeout: Duration = .seconds(5)) async {
        let dir = directory(appId: appId, root: root)
        let targets = live.filter { $0.directory == dir }
        live.removeAll { $0.directory == dir || ($0.engine == nil && $0.queue == nil) }
        let deadline = ContinuousClock.now + timeout
        for target in targets {
            if let engine = target.engine { await engine.wipe() } else { await target.queue?.discardAll() }
            while let queue = target.queue, await queue.isDraining, ContinuousClock.now < deadline {
                try? await Task.sleep(for: .milliseconds(20))
            }
        }
        try? FileManager.default.removeItem(at: dir)
    }
}
