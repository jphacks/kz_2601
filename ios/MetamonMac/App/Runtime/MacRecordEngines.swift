import Foundation
import MetamonServices

/// 作ったアプリの記録のエンジンを appId ごとに 1 つだけ持つ(`MacRecordVault` の同じフォルダを 2 つのエンジンで読み書きしない)。
/// `RecordStore` は entity の表をメモリに持って毎回ファイルごと書くので、2 つのウインドウが別々のエンジンを持つと
/// 後から保存した方がもう片方の足した行を消してしまう。ウインドウは `acquire` で借り、閉じたら `release` で返す。
/// 最後のウインドウが返したら止めて手放す。通知(`onEvent`)は借りている全員に配る。
@MainActor
final class MacRecordEngines {
    static let shared = MacRecordEngines()

    private let root: URL
    private var entries: [String: Entry] = [:]

    init(root: URL = MacRecordVault.root) {
        self.root = root
    }

    @MainActor
    fileprivate final class Entry {
        let engine: RecordSyncEngine
        var leases: [ObjectIdentifier: MacRecordEngineLease] = [:]

        init(engine: RecordSyncEngine) { self.engine = engine }

        func broadcast(_ event: SyncEvent) {
            // 配っている途中で返されても大丈夫なよう写しを回す
            for lease in Array(leases.values) { lease.onEvent?(event) }
        }
    }

    /// その appId のエンジンを借りる。まだ無ければ作って動かし始める。`server` は最初に作る時だけ使う
    func acquire(appId: String, server: (any AppConfigClient)? = nil) -> MacRecordEngineLease {
        let entry: Entry
        if let existing = entries[appId] {
            entry = existing
        } else {
            let engine = MacRecordVault.recordSync(appId: appId, server: server, root: root)
            entry = Entry(engine: engine)
            engine.onEvent = { [weak entry] event in entry?.broadcast(event) }
            engine.start()
            entries[appId] = entry
        }
        let lease = MacRecordEngineLease(appId: appId, engine: entry.engine, owner: self)
        entry.leases[ObjectIdentifier(lease)] = lease
        return lease
    }

    /// 借りているエンジン(テスト用)
    func engine(appId: String) -> RecordSyncEngine? { entries[appId]?.engine }

    /// 借りている数(テスト用)
    func leaseCount(appId: String) -> Int { entries[appId]?.leases.count ?? 0 }

    fileprivate func release(_ lease: MacRecordEngineLease) {
        // アプリを消した後に開いていたウインドウが返す時は、もう別のエントリ(作り直したもの)かもしれない
        guard let entry = entries[lease.appId], entry.engine === lease.engine else { return }
        entry.leases[ObjectIdentifier(lease)] = nil
        guard entry.leases.isEmpty else { return }
        entry.engine.stop()
        entry.engine.onEvent = nil
        entries[lease.appId] = nil
    }

    /// アプリを消す時: 借りているウインドウが残っていてもエンジンを手放し(通知も外す)、`MacRecordVault.purge` で
    /// 止めて送信待ちとメモリ上の表を空にし、送っている途中の 1 件を待ってからフォルダを消す。
    /// 止める前に消すと、動いているエンジンが表を書き戻してフォルダを作り直してしまう
    func purge(appId: String) async {
        let entry = entries.removeValue(forKey: appId)
        if let entry {
            entry.engine.stop()
            entry.engine.onEvent = nil
            entry.leases.values.forEach { $0.detach() }
        }
        await MacRecordVault.purge(appId: appId, root: root)
        withExtendedLifetime(entry) {} // 消し終わるまでエンジンを生かしておく(弱く持たれているので)
    }
}

/// `MacRecordEngines` から借りたエンジン。ウインドウ(ランタイム)ごとに 1 つ。`release` は何度呼んでも 1 回だけ
@MainActor
final class MacRecordEngineLease {
    let appId: String
    let engine: RecordSyncEngine
    /// このウインドウへの同期の通知
    var onEvent: (@MainActor (SyncEvent) -> Void)?
    private weak var owner: MacRecordEngines?

    fileprivate init(appId: String, engine: RecordSyncEngine, owner: MacRecordEngines) {
        self.appId = appId
        self.engine = engine
        self.owner = owner
    }

    func release() {
        guard let owner else { return }
        self.owner = nil
        onEvent = nil
        owner.release(self)
    }

    /// アプリを消した(`MacRecordEngines.purge`)。もう返す先は無い
    fileprivate func detach() {
        owner = nil
        onEvent = nil
    }
}
