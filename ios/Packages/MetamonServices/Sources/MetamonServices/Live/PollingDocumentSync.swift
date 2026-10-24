import Foundation
import MetamonDSL

/// ポーリング方式のホットリロード(v1)。version が進んだ時だけ流す。
public struct PollingDocumentSync: DocumentSync {
    public let client: any AppConfigClient
    public var interval: Duration

    public init(client: any AppConfigClient, interval: Duration = .seconds(2)) {
        self.client = client
        self.interval = interval
    }

    public func updates(appId: String, after version: Int) -> AsyncStream<AppDocument> {
        AsyncStream { continuation in
            let task = Task {
                var last = version
                while !Task.isCancelled {
                    try? await Task.sleep(for: interval)
                    if let doc = try? await client.fetchDocument(appId: appId), doc.version > last {
                        last = doc.version
                        continuation.yield(doc)
                    }
                }
                continuation.finish()
            }
            continuation.onTermination = { _ in task.cancel() }
        }
    }
}
