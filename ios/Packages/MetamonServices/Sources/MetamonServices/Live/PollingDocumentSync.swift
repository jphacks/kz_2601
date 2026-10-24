import Foundation
import MetamonDSL

/// ポーリング方式のホットリロード(Day1 版・G13)。
/// `If-None-Match: "<version>"` を付けて `GET /dsl` し、304 なら何もしない。version が進んだ時だけ流す。
/// 前面時だけ回すのは呼び出し側(`AppRuntime.setForeground`)が stream を止める/張り直すことで実現する。
public struct PollingDocumentSync: DocumentSync {
    public let client: any AppConfigClient
    public var interval: Duration

    public init(client: any AppConfigClient, interval: Duration = .seconds(5)) {
        self.client = client
        self.interval = interval
    }

    public func updates(appId: String, after version: Int) -> AsyncStream<AppDocument> {
        AsyncStream { continuation in
            let task = Task {
                var last = version
                while !Task.isCancelled {
                    try? await Task.sleep(for: interval)
                    guard !Task.isCancelled else { break }
                    do {
                        if let doc = try await client.fetchDocumentIfChanged(appId: appId, currentVersion: last), doc.version > last {
                            last = doc.version
                            continuation.yield(doc)
                        }
                    } catch {
                        // 一時的な失敗は次の周期で再試行(ログだけ)
                        print("[Metamon] poll failed: \(error.localizedDescription)")
                    }
                }
                continuation.finish()
            }
            continuation.onTermination = { _ in task.cancel() }
        }
    }
}
