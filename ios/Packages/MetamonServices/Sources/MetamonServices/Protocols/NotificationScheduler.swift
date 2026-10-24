import Foundation
import MetamonDSL

/// ローカル通知(P0)。
/// - `sync`: `app.notifications[]` を DSL ロード毎に同期(id 一致で置換・消えた id は取消)。
/// - `schedule`: `scheduleNotification` アクションで押下時に予約(同 id なら上書き)。
public protocol NotificationScheduler: Sendable {
    func sync(_ specs: [NotificationSpec]) async
    func schedule(_ spec: NotificationSpec) async throws
    func cancel(id: String)
}
