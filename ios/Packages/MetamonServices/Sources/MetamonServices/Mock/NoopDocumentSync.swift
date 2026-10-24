import Foundation
import MetamonDSL

/// 何も流さないホットリロード(モック / オフライン用)。
public struct NoopDocumentSync: DocumentSync {
    public init() {}

    public func updates(appId: String, after version: Int) -> AsyncStream<AppDocument> {
        AsyncStream { $0.finish() }
    }
}
