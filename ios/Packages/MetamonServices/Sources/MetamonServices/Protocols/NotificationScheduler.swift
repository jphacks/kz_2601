import Foundation
import MetamonDSL

/// 次に届く通知(設定画面の表示用)。
public struct UpcomingNotification: Sendable, Equatable {
    public var title: String
    public var body: String
    public var date: Date
    public init(title: String, body: String, date: Date) {
        self.title = title
        self.body = body
        self.date = date
    }
}

/// `sync` の結果。`denied` は通知が許可されていない(予約しても届かない)。
public struct NotificationSyncResult: Sendable, Equatable {
    public var denied: Bool
    public var upcoming: [UpcomingNotification]
    public init(denied: Bool = false, upcoming: [UpcomingNotification] = []) {
        self.denied = denied
        self.upcoming = upcoming
    }
}

/// ローカル通知(#228)。アプリ(appId)ごとに予約を持ち、全アプリ分から近い順に iOS の上限内で予約し直す。
/// - `sync`: `app.notifications[]`(固定時刻 / レコードの時刻から)をそのアプリ分として置き換える。
/// - `schedule`: `scheduleNotification` アクション(押下時・同 id なら上書き)。DSL の再同期では消えない。
public protocol NotificationScheduler: Sendable {
    @discardableResult
    func sync(appId: String, specs: [NotificationSpec], records: [String: [Record]]) async -> NotificationSyncResult
    func schedule(_ spec: NotificationSpec, appId: String) async throws -> Date?
    func cancel(id: String, appId: String) async
    /// アプリ削除時: そのアプリの予約を消す。
    func removeApp(_ appId: String) async
    func removeAll() async
    /// 予約し直すだけ(1 回きりが届いて空いた枠を次の通知で埋める)。
    @discardableResult
    func refresh(appId: String) async -> NotificationSyncResult
}
