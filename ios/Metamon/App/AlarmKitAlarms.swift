import Foundation
import MetamonServices
#if !APP_STORE_REVIEW && canImport(AlarmKit)
import AlarmKit
import SwiftUI
#endif

/// 止めるまで鳴るアラーム(`Notification.alarm`)の実装を選ぶ。
/// iOS 26.1 未満(26.0 の Alert は廃止予定の止めるボタンが必須)・審査用の版は nil = 普通の通知で届ける。
enum AlarmBackend {
    static func make() -> (any AlarmScheduling)? {
        #if !APP_STORE_REVIEW && canImport(AlarmKit)
        if #available(iOS 26.1, *) { return AlarmKitAlarms() }
        #endif
        return nil
    }
}

#if !APP_STORE_REVIEW && canImport(AlarmKit)
/// AlarmKit による予約。許可の説明は Info.plist の `NSAlarmKitUsageDescription`(審査用の版は strip-review-plist.sh が消す)。
@available(iOS 26.1, *)
struct AlarmKitAlarms: AlarmScheduling {
    struct Metadata: AlarmMetadata {}

    func authorize() async -> Bool {
        let manager = AlarmManager.shared
        switch manager.authorizationState {
        case .authorized: return true
        case .denied: return false
        case .notDetermined: return (try? await manager.requestAuthorization()) == .authorized
        @unknown default: return false
        }
    }

    func scheduled() async -> [ScheduledAlarm] {
        ((try? AlarmManager.shared.alarms) ?? []).map { ScheduledAlarm(id: $0.id, alerting: $0.state == .alerting) }
    }

    func schedule(_ request: AlarmRequest) async throws {
        let alert = AlarmPresentation.Alert(title: "\(request.title)")
        let attributes = AlarmAttributes<Metadata>(presentation: AlarmPresentation(alert: alert), tintColor: .accentColor)
        _ = try await AlarmManager.shared.schedule(
            id: request.id, configuration: .alarm(schedule: Self.schedule(request.trigger), attributes: attributes))
    }

    func cancel(_ id: UUID) async {
        try? AlarmManager.shared.cancel(id: id)
    }

    static func schedule(_ trigger: AlarmTrigger) -> Alarm.Schedule {
        switch trigger {
        case .fixed(let date):
            return .fixed(date)
        case .relative(let hour, let minute, let weekdays):
            let days = weekdays.compactMap(weekday)
            return .relative(.init(time: .init(hour: hour, minute: minute), repeats: days.isEmpty ? .never : .weekly(days)))
        }
    }

    /// 1 = 日 〜 7 = 土(`Calendar` の weekday)
    static func weekday(_ n: Int) -> Locale.Weekday? {
        let all: [Locale.Weekday] = [.sunday, .monday, .tuesday, .wednesday, .thursday, .friday, .saturday]
        return all.indices.contains(n - 1) ? all[n - 1] : nil
    }
}
#endif
