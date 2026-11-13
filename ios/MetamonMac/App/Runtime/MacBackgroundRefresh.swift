import Foundation
import MetamonServices

/// ウインドウが閉じていても時々取り直す(#2253・iOS の `BGAppRefreshTask` に当たる)。
/// `NSBackgroundActivityScheduler` が iOS と同じ目安の間隔(`BackgroundRefreshCycle.interval`)・`.utility` で起こし、
/// 飛ばす条件と順番は iOS と共有の `BackgroundRefreshCycle.perform`:
/// サインアウト中・オフラインは何もしない → 未送信のキューを流す(`MacRecordResend`)→ 一覧と開いているアプリを取り直す。
@MainActor
final class MacBackgroundRefresh {
    static let identifier = "jp.kz2601.metamon.refresh"
    static let shared = MacBackgroundRefresh()

    var isSignedIn: @MainActor () -> Bool
    var isOnline: @MainActor () -> Bool
    var drain: @MainActor () async -> Void
    /// 一覧と開いているアプリの取り直し(`MetamonMacApp` が一覧を渡して組む)
    var refetch: @MainActor () async -> Void
    /// 何回取り直したか(テスト用)
    private(set) var refreshCount = 0

    private var scheduler: NSBackgroundActivityScheduler?
    private var running = false

    init(isSignedIn: @escaping @MainActor () -> Bool = { MacServices.shared.auth.currentUser != nil },
         isOnline: (@MainActor () -> Bool)? = nil,
         drain: @escaping @MainActor () async -> Void = { await MacBackgroundRefresh.drainAll(MacRecordResend.shared) },
         refetch: @escaping @MainActor () async -> Void = {}) {
        self.isSignedIn = isSignedIn
        if let isOnline {
            self.isOnline = isOnline
        } else {
            let connectivity = NetworkPathConnectivity()
            self.isOnline = { connectivity.isOnline }
        }
        self.drain = drain
        self.refetch = refetch
    }

    /// 起動時に 1 回。2 回呼んでも 1 つだけ
    func install() {
        guard scheduler == nil else { return }
        let scheduler = NSBackgroundActivityScheduler(identifier: Self.identifier)
        scheduler.repeats = true
        scheduler.interval = BackgroundRefreshCycle.interval
        scheduler.tolerance = BackgroundRefreshCycle.interval / 3
        scheduler.qualityOfService = .utility
        scheduler.schedule { [weak self] completion in
            Task { @MainActor in
                await self?.run()
                completion(.finished)
            }
        }
        self.scheduler = scheduler
    }

    func uninstall() {
        scheduler?.invalidate()
        scheduler = nil
    }

    /// 1 回分。前の回がまだ走っていれば重ねない
    @discardableResult
    func run() async -> BackgroundRefreshCycle.Outcome? {
        guard !running else { return nil }
        running = true
        defer { running = false }
        let outcome = await BackgroundRefreshCycle.perform(reason: "mac-bg-refresh", signedIn: isSignedIn(), online: isOnline(),
                                                           drain: drain, refetch: refetch)
        if outcome == .refreshed { refreshCount += 1 }
        return outcome
    }

    /// 全部のエンジンの送信待ちを流し、送り終わる(空・止まる・オフライン)まで待つ
    static func drainAll(_ resend: MacRecordResend) async {
        for engine in resend.engines() { await resend.resend(engine) }
    }
}
