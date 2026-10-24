import Foundation
import MetamonDSL

/// ホットリロード: 新しい version の DSL を流す。
public protocol DocumentSync: Sendable {
    func updates(appId: String, after version: Int) -> AsyncStream<AppDocument>
}
