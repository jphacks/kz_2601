import Foundation
import Observation

/// 1 つのアプリのフォロー一覧(#270)。操作は楽観的に先に反映し、失敗したら戻して throw する。サーバーの応答が正。
@MainActor
@Observable
public final class FollowStore {
    public let appId: String
    private let client: any FollowsClient
    /// フォローしている人。nil = まだ読めていない
    public private(set) var uids: Set<String>?
    /// 送信中の相手(ボタンを押せなくする)
    public private(set) var busy: Set<String> = []
    @ObservationIgnored private var loading = false

    public init(appId: String, client: any FollowsClient) {
        self.appId = appId
        self.client = client
    }

    /// 読み込む(読み込み中の 2 回目は何もしない)。失敗したら前の一覧のまま throw。
    public func load() async throws {
        guard !loading else { return }
        loading = true
        defer { loading = false }
        uids = Set(try await client.list(appId: appId))
    }

    public func isFollowing(_ uid: String) -> Bool { uids?.contains(uid) ?? false }

    public func set(_ uid: String, on: Bool) async throws {
        guard !busy.contains(uid) else { return }
        let before = uids
        apply(uid, on: on)
        busy.insert(uid)
        defer { busy.remove(uid) }
        do {
            apply(uid, on: try await client.setFollow(appId: appId, uid: uid, on: on))
        } catch {
            uids = before
            throw error
        }
    }

    private func apply(_ uid: String, on: Bool) {
        var next = uids ?? []
        if on { next.insert(uid) } else { next.remove(uid) }
        uids = next
    }
}
