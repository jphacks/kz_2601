import Foundation
import Network

/// 音声モデルのダウンロード確認に添える注意(#275): モバイル回線・空き容量不足。
public struct ModelDownloadAdvice: Equatable, Sendable {
    /// モバイル回線(または省データモード)につながっている
    public var isExpensiveNetwork: Bool
    /// 空き容量が足りない時の不足分(MB)。足りていれば nil
    public var shortageMegabytes: Int?

    public var isLowStorage: Bool { shortageMegabytes != nil }

    public init(isExpensiveNetwork: Bool, shortageMegabytes: Int?) {
        self.isExpensiveNetwork = isExpensiveNetwork
        self.shortageMegabytes = shortageMegabytes
    }

    /// ダウンロードと展開に要る空き容量の目安(モデルの約 2 倍)。
    public static func requiredBytes(megabytes: Int) -> Int64 {
        Int64(megabytes) * 2 * 1_000_000
    }

    public static func evaluate(megabytes: Int, isExpensiveNetwork: Bool, availableBytes: Int64?) -> ModelDownloadAdvice {
        var shortage: Int?
        if let availableBytes {
            let missing = requiredBytes(megabytes: megabytes) - availableBytes
            if missing > 0 { shortage = Int((missing + 999_999) / 1_000_000) }
        }
        return ModelDownloadAdvice(isExpensiveNetwork: isExpensiveNetwork, shortageMegabytes: shortage)
    }

    /// 端末の空き容量(大事なデータ用に OS が空けられる分を含む)。取れなければ nil。
    public static func availableBytesOnDevice() -> Int64? {
        let url = URL(fileURLWithPath: NSHomeDirectory())
        return (try? url.resourceValues(forKeys: [.volumeAvailableCapacityForImportantUsageKey]))?
            .volumeAvailableCapacityForImportantUsage
    }
}

/// いまの回線が従量(モバイル回線・テザリング・省データモード)か。起動時から監視しておく。
public final class ExpensiveNetworkProbe: @unchecked Sendable {
    public static let shared = ExpensiveNetworkProbe()

    private let monitor = NWPathMonitor()
    private let lock = NSLock()
    private var expensive = false

    init() {
        monitor.pathUpdateHandler = { [weak self] path in
            guard let self else { return }
            let now = path.isExpensive || path.isConstrained
            self.lock.withLock { self.expensive = now }
        }
        monitor.start(queue: DispatchQueue(label: "jp.kz2601.metamon.network-cost"))
    }

    deinit { monitor.cancel() }

    public var isExpensive: Bool { lock.withLock { expensive } }
}
