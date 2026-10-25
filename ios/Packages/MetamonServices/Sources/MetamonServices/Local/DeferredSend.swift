import Foundation

/// 圏外で送れなかった **1 件** を保持し、オンライン復帰で **1 回だけ** 流す(#60 の「接続が戻ったら自動で送ります」)。
/// 新しい発話が来たら前の保留は置き換える(溜めない)。`cancel()` で取り消せる。
/// `AppRuntime` が生成(morph)に使う。到達性は `ConnectivityMonitor`(本番は NWPathMonitor、E2E/テストは `ManualConnectivity`)。
public final class DeferredSend<Payload: Sendable>: @unchecked Sendable {
    private let lock = NSLock()
    private let connectivity: any ConnectivityMonitor
    private var pendingPayload: Payload?
    private var waiter: Task<Void, Never>?
    private var generation = 0

    public init(connectivity: any ConnectivityMonitor) {
        self.connectivity = connectivity
    }

    public var isOnline: Bool { connectivity.isOnline }

    /// 保留中の 1 件。
    public var pending: Payload? { lock.withLock { pendingPayload } }

    /// オンラインなら false を返して何もしない(呼び出し側が即送る)。
    /// オフラインなら保留して true を返し、復帰した時に `send` を 1 回呼ぶ(保留が差し替え/取消されていれば呼ばない)。
    @discardableResult
    public func holdIfOffline(_ payload: Payload, send: @escaping @Sendable (Payload) async -> Void) -> Bool {
        guard !connectivity.isOnline else { return false }
        let gen: Int = lock.withLock {
            generation += 1
            pendingPayload = payload
            waiter?.cancel()
            return generation
        }
        let task = Task { [connectivity] in
            for await online in connectivity.updates() {
                if Task.isCancelled { return }
                guard online else { continue }
                let fire: Payload? = self.lock.withLock {
                    guard self.generation == gen, let p = self.pendingPayload else { return nil }
                    self.pendingPayload = nil
                    self.waiter = nil
                    return p
                }
                if let fire { await send(fire) }
                return
            }
        }
        lock.withLock { waiter = task }
        return true
    }

    /// 保留を取り消す(復帰しても送らない)。
    public func cancel() {
        lock.withLock {
            generation += 1
            pendingPayload = nil
            waiter?.cancel()
            waiter = nil
        }
    }

    deinit { waiter?.cancel() }
}
