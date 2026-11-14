import Foundation
import UserNotifications

/// `UNUserNotificationCenter` の使う分だけ(テストで差し替える)。
protocol NotificationCenterClient: Sendable {
    func pendingIdentifiers() async -> [String]
    func add(_ request: UNNotificationRequest) async throws
    func removePending(_ identifiers: [String]) async
    func authorizationStatus() async -> UNAuthorizationStatus
    func requestAuthorization() async -> Bool
}

struct SystemNotificationCenter: NotificationCenterClient {
    private var center: UNUserNotificationCenter { .current() }
    func pendingIdentifiers() async -> [String] { await center.pendingNotificationRequests().map(\.identifier) }
    func add(_ request: UNNotificationRequest) async throws { try await center.add(request) }
    func removePending(_ identifiers: [String]) async { center.removePendingNotificationRequests(withIdentifiers: identifiers) }
    func authorizationStatus() async -> UNAuthorizationStatus { await center.notificationSettings().authorizationStatus }
    func requestAuthorization() async -> Bool { (try? await center.requestAuthorization(options: [.alert, .sound, .badge])) ?? false }
}

/// 場所の通知に要る位置情報の許可と最後の位置(テストで差し替える)。
struct RegionAccess: Sendable {
    var authorize: @Sendable () async -> Bool
    var isAuthorized: @Sendable () async -> Bool
    var lastKnown: @Sendable () async -> (lat: Double, lng: Double)?

    static var system: RegionAccess {
        #if os(iOS)
        RegionAccess(authorize: { await RegionLocationAccess.shared.authorize() },
                     isAuthorized: { await RegionLocationAccess.shared.isAuthorized },
                     lastKnown: { await RegionLocationAccess.shared.lastKnown })
        #else
        RegionAccess(authorize: { false }, isAuthorized: { false }, lastKnown: { nil })
        #endif
    }
}

/// 非同期の処理を 1 本ずつ順に流す。actor だけでは await の間に次の呼び出しが割り込むため。
actor AsyncSerialQueue {
    /// 通知の予約し直し(全アプリ・全インスタンスで共有)。
    static let notifications = AsyncSerialQueue()
    private var tail: Task<Void, Never>?

    func run<T: Sendable>(_ op: @escaping @Sendable () async -> T) async -> T {
        let previous = tail
        let task = Task { () -> T in
            await previous?.value
            return await op()
        }
        tail = Task { _ = await task.value }
        return await task.value
    }
}
