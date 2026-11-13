import AppKit
import Foundation
import MetamonServices

/// 記録の送信待ちを Mac で送り直す合図(#976)。送信待ちの列そのものは iOS と同じ `RecordSyncEngine` / `SyncQueue`
/// (圏外・サーバーの失敗で積み、順に 1 件ずつ・同じ記録は畳む・live/queue.json に残る)。
/// iOS の「前面に戻った」の代わりに、次で全部のエンジンを流し直す:
/// - ウインドウが前に来た(`didBecomeActive`)・スリープから戻った(`didWake`)・ネットワークが戻った
/// - 起動中は一定間隔(連続失敗で止まった列も、ここで再開する)
/// 送っている間は App Nap で止まらないよう `ProcessInfo` の activity を持つ。スリープの前は取り消し待ちの削除を列に積む。
@MainActor
final class MacRecordResend {
    static let shared = MacRecordResend()

    /// 流し直すエンジン(既定は開いている全部のアプリ)
    var engines: @MainActor () -> [RecordSyncEngine]
    /// App Nap を止める印
    let activity: MacSendActivity
    /// 送り終わるまで待つ上限(1 回の送り直しで)
    var timeout: Duration
    /// 同じ合図で他の端末の変化の購読を張り直す(#2072・`MacRealtimeSync.resume`)。
    /// `force`: スリープ明け・回線の復帰(購読が生きて見えても張り直す)。false はウインドウが前に来ただけ
    var onResume: (@MainActor (_ force: Bool) -> Void)?
    /// 何回送り直したか(テスト用)
    private(set) var resendCount = 0

    private var running: Set<ObjectIdentifier> = []
    private var observers: [(NotificationCenter, NSObjectProtocol)] = []
    private var tasks: [Task<Void, Never>] = []

    init(engines: @escaping @MainActor () -> [RecordSyncEngine] = { MacRecordVault.liveEngines },
         activity: MacSendActivity? = nil, timeout: Duration = .seconds(30)) {
        self.engines = engines
        self.activity = activity ?? MacSendActivity()
        self.timeout = timeout
    }

    /// 起動時に 1 回。2 回呼んでも 1 回だけ
    func install(connectivity: any ConnectivityMonitor = NetworkPathConnectivity(), interval: Duration = .seconds(60)) {
        guard observers.isEmpty else { return }
        let app = NotificationCenter.default
        let workspace = NSWorkspace.shared.notificationCenter
        observe(app, NSApplication.didBecomeActiveNotification) { $0.resume(force: false) }
        observe(workspace, NSWorkspace.didWakeNotification) { $0.resume(force: true) }
        observe(workspace, NSWorkspace.screensDidWakeNotification) { $0.resume(force: true) }
        observe(workspace, NSWorkspace.willSleepNotification) { $0.prepareForSleep() }
        tasks.append(Task { [weak self] in
            var wasOnline = true
            for await online in connectivity.updates() {
                if online, !wasOnline {
                    try? await Task.sleep(for: .seconds(1)) // 各エンジンの到達性が追いつくのを待つ
                    self?.resume(force: true)
                }
                wasOnline = online
            }
        })
        tasks.append(Task { [weak self] in
            while !Task.isCancelled {
                try? await Task.sleep(for: interval, tolerance: .seconds(10)) // App Nap 中にまとめて起こされてよい
                guard let self else { return }
                if self.engines().contains(where: { $0.pendingCount > 0 }) { self.resendAll() }
            }
        })
    }

    private func observe(_ center: NotificationCenter, _ name: Notification.Name, _ body: @escaping @MainActor (MacRecordResend) -> Void) {
        let token = center.addObserver(forName: name, object: nil, queue: .main) { [weak self] _ in
            MainActor.assumeIsolated { if let self { body(self) } }
        }
        observers.append((center, token))
    }

    func uninstall() {
        observers.forEach { $0.0.removeObserver($0.1) }
        observers = []
        tasks.forEach { $0.cancel() }
        tasks = []
    }

    /// 前面・スリープ明け・回線の復帰: 送信待ちを流し直し、購読も張り直す
    func resume(force: Bool) {
        resendAll()
        onResume?(force)
    }

    /// 全部のエンジンの送信待ちを流し直す(待たない)
    func resendAll() {
        for engine in engines() { Task { await resend(engine) } }
    }

    /// 1 つのエンジンの送信待ちを流し直し、空になる・止まる・オフラインになるまで待つ。その間は App Nap を止める。
    /// 同じエンジンで同時に 2 回は走らない(列も 1 本ずつしか送らない・二重には送らない)
    func resend(_ engine: RecordSyncEngine) async {
        let key = ObjectIdentifier(engine)
        guard engine.isOnline, !running.contains(key) else { return }
        running.insert(key) // 数える間に次の合図が来ても 2 本目を始めない
        defer { running.remove(key) }
        guard await engine.queue.count > 0 else { return }
        resendCount += 1
        activity.begin()
        defer { activity.end() }
        engine.setForeground(true) // 止まった列も再開して流す(iOS の前面復帰と同じ)
        let clock = ContinuousClock()
        let deadline = clock.now + timeout
        while engine.isOnline, clock.now < deadline {
            try? await Task.sleep(for: .milliseconds(50))
            let queue = engine.queue
            if await queue.isDraining { continue }
            let left = await queue.count
            let paused = await queue.isPaused
            if left == 0 || paused { break }
        }
    }

    /// スリープの前: 取り消し待ちの削除を列に積む(寝ている間に消えないように)
    func prepareForSleep() {
        for engine in engines() { engine.setForeground(false) }
    }
}

/// 送っている間だけ App Nap を止める(`ProcessInfo.beginActivity`)。何本同時でも 1 つの activity にまとめる
@MainActor
final class MacSendActivity {
    private let beginActivity: @MainActor () -> NSObjectProtocol
    private let endActivity: @MainActor (NSObjectProtocol) -> Void
    private var token: NSObjectProtocol?
    private(set) var depth = 0

    init(begin: @escaping @MainActor () -> NSObjectProtocol = {
        ProcessInfo.processInfo.beginActivity(options: [.userInitiatedAllowingIdleSystemSleep],
                                              reason: "Sending records to the server")
    }, end: @escaping @MainActor (NSObjectProtocol) -> Void = { ProcessInfo.processInfo.endActivity($0) }) {
        beginActivity = begin
        endActivity = end
    }

    var isActive: Bool { token != nil }

    func begin() {
        depth += 1
        if token == nil { token = beginActivity() }
    }

    func end() {
        guard depth > 0 else { return }
        depth -= 1
        if depth == 0, let token {
            endActivity(token)
            self.token = nil
        }
    }
}
