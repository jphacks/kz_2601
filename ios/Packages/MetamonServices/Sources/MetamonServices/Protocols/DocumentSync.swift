import Foundation
import MetamonDSL

/// ホットリロード: 新しい version の DSL を流す。
public protocol DocumentSync: Sendable {
    func updates(appId: String, after version: Int) -> AsyncStream<AppDocument>
    /// DSL に加えて「records が変わった」「リアルタイム購読の稼働/停止」も流す(G13 Day2・#82)。
    /// 既定実装は `updates` を `.document` に包むだけ(ポーリング / Noop はこれで足りる)。
    func events(appId: String, after version: Int) -> AsyncStream<SyncUpdate>
}

/// `DocumentSync.events` が流すもの。
public enum SyncUpdate: Sendable {
    /// version が進んだ DSL(appconfig から取り直した本体)
    case document(AppDocument)
    /// この entity の records が他端末 / 他ユーザーで変わった(本体は呼び出し側が API で取り直す)
    case records(entity: String)
    /// リアルタイム購読が稼働した(true)/ 切れてポーリングに落ちた(false)。値が変わった時だけ流す
    case realtime(Bool)
}

public extension DocumentSync {
    func events(appId: String, after version: Int) -> AsyncStream<SyncUpdate> {
        let docs = updates(appId: appId, after: version)
        return AsyncStream { continuation in
            let task = Task {
                for await doc in docs { continuation.yield(.document(doc)) }
                continuation.finish()
            }
            continuation.onTermination = { _ in task.cancel() }
        }
    }
}
