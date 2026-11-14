import Foundation
import MetamonRenderer
import MetamonServices

/// アプリを消した時の通知の後片付け。iOS(#228)と Mac(#1324)で同じ。
/// - DSL の通知・条件の通知・場所の通知の予約(識別子 `dsl.<appId>.…` ほか)
/// - タイマーの終了通知と走行状態(キーが `<appId>/` で始まる。同じ名前の別のアプリは残る)
enum AppNotificationCleanup {
    @MainActor
    static func removeApp(_ appId: String, notifications: any NotificationScheduler) async {
        await notifications.removeApp(appId)
        await TimerCleanup.removeApp(appId: appId)
    }
}
