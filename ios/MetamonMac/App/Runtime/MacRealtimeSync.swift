import Foundation
import MetamonDSL
import MetamonServices

/// iPhone など別の端末で変わった記録・アプリの版を、開いているアプリにすぐ出す(#2072)。
/// 購読そのものは iOS と同じ `DocumentSync`(`RealtimeDocumentSync`: SSE `GET /events`・切れたらポーリング)。
/// - appId ごとに購読は 1 本。ウインドウ(ランタイム)は `acquire` で借り、閉じたら `release`。最後が返したら購読をやめる
/// - 記録の変化: entity ごとに `debounce` だけまとめてから借りている全員へ(連投で取り直しを重ねない)
/// - 版の変化: `onDocument` へ。どれかのウインドウが入力・操作の途中なら、終わるまで待って最新の 1 件だけ渡す(iOS の hotReload と同じ)
/// - 購読が切れて戻った・スリープ明け・回線の復帰: 表示中の entity を取り直す(切れていた間の取りこぼし)
/// 購読している間は App Nap で止まらないよう activity を持つ(`MacSendActivity`)。
@MainActor
final class MacRealtimeSync {
    static let shared = MacRealtimeSync(sync: MacServices.shared.sync)

    /// nil(モック・appconfig の URL 無し)なら購読しない
    let sync: (any DocumentSync)?
    let activity: MacSendActivity
    var debounce: Duration
    /// 入力中で保留した版を当てられるか確かめる間隔
    var deferInterval: Duration
    /// 新しい版の DSL(既定は `MetamonMacApp` が一覧に当てる)
    var onDocument: (@MainActor (AppDocument) -> Void)?

    private var entries: [String: Entry] = [:]

    init(sync: (any DocumentSync)?, activity: MacSendActivity? = nil,
         debounce: Duration = .milliseconds(250), deferInterval: Duration = .seconds(1)) {
        self.sync = sync
        self.activity = activity ?? MacSendActivity(begin: {
            ProcessInfo.processInfo.beginActivity(options: [.userInitiatedAllowingIdleSystemSleep],
                                                  reason: "Listening for changes from other devices")
        })
        self.debounce = debounce
        self.deferInterval = deferInterval
    }

    @MainActor
    fileprivate final class Entry {
        var version: Int
        var listening: Bool?
        var stream: Task<Void, Never>?
        var leases: [ObjectIdentifier: MacRealtimeLease] = [:]
        var refreshTasks: [String: Task<Void, Never>] = [:]
        var pending: AppDocument?
        var pendingTask: Task<Void, Never>?

        init(version: Int) { self.version = version }

        func cancelAll() {
            stream?.cancel()
            stream = nil
            refreshTasks.values.forEach { $0.cancel() }
            refreshTasks = [:]
            pendingTask?.cancel()
            pendingTask = nil
            pending = nil
        }
    }

    /// その appId の購読を借りる。まだ無ければ `version` の次から購読を始める。`sync` が無ければ nil
    func acquire(appId: String, version: Int) -> MacRealtimeLease? {
        guard sync != nil else { return nil }
        let entry: Entry
        if let existing = entries[appId] {
            entry = existing
            entry.version = max(entry.version, version)
        } else {
            entry = Entry(version: version)
            entries[appId] = entry
            activity.begin()
            start(appId, entry)
        }
        let lease = MacRealtimeLease(appId: appId, owner: self)
        entry.leases[ObjectIdentifier(lease)] = lease
        return lease
    }

    fileprivate func release(_ lease: MacRealtimeLease) {
        guard let entry = entries[lease.appId], entry.leases[ObjectIdentifier(lease)] === lease else { return }
        entry.leases[ObjectIdentifier(lease)] = nil
        guard entry.leases.isEmpty else { return }
        stop(lease.appId)
    }

    /// サインアウト・アカウント削除: 全部の購読をやめる(借りているウインドウには何も届かなくなる)
    func stopAll() {
        for appId in Array(entries.keys) { stop(appId) }
    }

    /// スリープ明け・回線の復帰(`force`)・ウインドウが前に来た: 張り直して表示中の entity を取り直す。
    /// 前に来ただけの時は、購読が生きていれば何も取りこぼしていないので張り直さない
    func resume(force: Bool) {
        for (appId, entry) in entries where force || entry.listening != true {
            start(appId, entry)
            broadcastCatchUp(entry)
        }
    }

    /// 購読している(ウインドウで開いている)アプリ
    var openAppIds: [String] { entries.keys.sorted() }

    /// 裏の定期の取り直し(#2253): 購読は張り直さず、開いている全部のアプリの表示中の entity を取り直す
    func catchUpAll() {
        for entry in entries.values { broadcastCatchUp(entry) }
    }

    // MARK: - テスト用

    func isSubscribed(_ appId: String) -> Bool { entries[appId]?.stream != nil }
    func leaseCount(appId: String) -> Int { entries[appId]?.leases.count ?? 0 }

    // MARK: - 購読

    private func stop(_ appId: String) {
        guard let entry = entries.removeValue(forKey: appId) else { return }
        entry.cancelAll()
        entry.leases.values.forEach { $0.detach() }
        activity.end()
    }

    private func start(_ appId: String, _ entry: Entry) {
        guard let sync else { return }
        entry.stream?.cancel()
        entry.listening = nil
        let updates = sync.events(appId: appId, after: entry.version)
        entry.stream = Task { [weak self, weak entry] in
            for await update in updates {
                guard let self, let entry, !Task.isCancelled else { return }
                self.handle(update, appId: appId, entry: entry)
            }
        }
    }

    private func handle(_ update: SyncUpdate, appId: String, entry: Entry) {
        switch update {
        case .records(let entity):
            scheduleRefresh(entity, entry: entry)
        case .document(let doc):
            guard doc.appId == appId, doc.version > entry.version else { return }
            entry.version = doc.version
            offer(doc, entry: entry)
        case .realtime(let on):
            let resumed = on && entry.listening == false
            entry.listening = on
            print("[Metamon] Mac realtime(\(appId)): \(on ? "listening" : "lost → polling")")
            if resumed { broadcastCatchUp(entry) } // 切れていた間の変化を取り直す
        }
    }

    private func scheduleRefresh(_ entity: String, entry: Entry) {
        entry.refreshTasks[entity]?.cancel()
        let debounce = debounce
        entry.refreshTasks[entity] = Task { [weak entry] in
            try? await Task.sleep(for: debounce)
            guard let entry, !Task.isCancelled else { return }
            entry.refreshTasks[entity] = nil
            for lease in Array(entry.leases.values) { lease.onRecordsChanged?(entity) }
        }
    }

    private func broadcastCatchUp(_ entry: Entry) {
        for lease in Array(entry.leases.values) { lease.onCatchUp?() }
    }

    // MARK: - 版の差し替え(入力中は待つ)

    private func isInteracting(_ entry: Entry) -> Bool {
        entry.leases.values.contains { $0.isInteracting?() ?? false }
    }

    private func offer(_ doc: AppDocument, entry: Entry) {
        guard isInteracting(entry) else {
            entry.pending = nil
            onDocument?(doc)
            return
        }
        if doc.version > (entry.pending?.version ?? 0) { entry.pending = doc }
        print("[Metamon] Mac realtime: hot reload deferred (interacting): version \(doc.version)")
        guard entry.pendingTask == nil else { return }
        let interval = deferInterval
        entry.pendingTask = Task { [weak self, weak entry] in
            while !Task.isCancelled {
                try? await Task.sleep(for: interval)
                guard let self, let entry, !Task.isCancelled else { return }
                guard let pending = entry.pending else { break }
                guard !self.isInteracting(entry) else { continue }
                entry.pending = nil
                entry.pendingTask = nil
                self.onDocument?(pending)
                return
            }
            entry?.pendingTask = nil
        }
    }
}

/// `MacRealtimeSync` から借りた購読。ウインドウ(ランタイム)ごとに 1 つ。`release` は何度呼んでも 1 回だけ
@MainActor
final class MacRealtimeLease {
    let appId: String
    /// 他の端末でこの entity の記録が変わった(まとめた後)
    var onRecordsChanged: (@MainActor (String) -> Void)?
    /// 購読が切れていた・スリープしていた。表示中の entity を全部取り直す
    var onCatchUp: (@MainActor () -> Void)?
    /// 入力・操作の途中か(版の差し替えを待つ)
    var isInteracting: (@MainActor () -> Bool)?
    private weak var owner: MacRealtimeSync?

    fileprivate init(appId: String, owner: MacRealtimeSync) {
        self.appId = appId
        self.owner = owner
    }

    func release() {
        guard let owner else { return }
        self.owner = nil
        owner.release(self)
        clear()
    }

    fileprivate func detach() {
        owner = nil
        clear()
    }

    private func clear() {
        onRecordsChanged = nil
        onCatchUp = nil
        isInteracting = nil
    }
}
