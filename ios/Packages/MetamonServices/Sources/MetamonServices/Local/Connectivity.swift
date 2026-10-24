import Foundation
import Network

/// ネットワーク到達性。`SyncQueue` がオンライン復帰を検知して未送信を流すために使う。
public protocol ConnectivityMonitor: Sendable {
    var isOnline: Bool { get }
    /// 現在値を最初に 1 回流し、以後は変化のたびに流す。
    func updates() -> AsyncStream<Bool>
}

/// `NWPathMonitor` 実装。`path.status == .satisfied` をオンラインとする。
public final class NetworkPathConnectivity: ConnectivityMonitor, @unchecked Sendable {
    private let monitor = NWPathMonitor()
    private let lock = NSLock()
    private var online = true
    private var continuations: [UUID: AsyncStream<Bool>.Continuation] = [:]

    public init() {
        monitor.pathUpdateHandler = { [weak self] path in
            guard let self else { return }
            let now = path.status == .satisfied
            let conts: [AsyncStream<Bool>.Continuation] = self.lock.withLock {
                guard self.online != now else { return [] }
                self.online = now
                return Array(self.continuations.values)
            }
            conts.forEach { $0.yield(now) }
        }
        monitor.start(queue: DispatchQueue(label: "jp.kz2601.metamon.connectivity"))
    }

    deinit { monitor.cancel() }

    public var isOnline: Bool { lock.withLock { online } }

    public func updates() -> AsyncStream<Bool> {
        AsyncStream { cont in
            let id = UUID()
            let current: Bool = lock.withLock { continuations[id] = cont; return online }
            cont.yield(current)
            cont.onTermination = { [weak self] _ in self?.lock.withLock { self?.continuations[id] = nil } }
        }
    }
}

/// テスト / E2E 用: 手動で切り替える到達性(`METAMON_E2E_OFFLINE_SECONDS` の機内モード擬似にも使う)。
public final class ManualConnectivity: ConnectivityMonitor, @unchecked Sendable {
    private let lock = NSLock()
    private var online: Bool
    private var continuations: [UUID: AsyncStream<Bool>.Continuation] = [:]

    public init(online: Bool = true) { self.online = online }

    public var isOnline: Bool { lock.withLock { online } }

    public func set(online now: Bool) {
        let conts: [AsyncStream<Bool>.Continuation] = lock.withLock {
            guard online != now else { return [] }
            online = now
            return Array(continuations.values)
        }
        conts.forEach { $0.yield(now) }
    }

    public func updates() -> AsyncStream<Bool> {
        AsyncStream { cont in
            let id = UUID()
            let current: Bool = lock.withLock { continuations[id] = cont; return online }
            cont.yield(current)
            cont.onTermination = { [weak self] _ in self?.lock.withLock { self?.continuations[id] = nil } }
        }
    }
}
