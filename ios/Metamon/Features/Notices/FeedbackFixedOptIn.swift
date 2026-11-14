// 審査用ビルド(APP_STORE_REVIEW)ではフィードバックと一緒に外す(#374)。
#if !APP_STORE_REVIEW
import Foundation
import UserNotifications
import MetamonServices

/// フィードバックの画面の「直ったら知らせてほしい」(#374)。既定はオフ。
/// - オンにした時だけ通知の許可を聞く。まだ決まっていない時だけ OS の確認を出す(許可済み・拒否済みなら出さない)
/// - 送った後に同意を保存する(`PUT /v1/me/notify-consent/feedback-fixed`)
@MainActor
struct FeedbackFixedOptIn {
    let client: any LoopNoticesClient
    var authorization: () async -> UNAuthorizationStatus
    var requestAuthorization: () async -> Bool
    /// 許可された後の端末の登録(既存の push の流れ・`PushHost`)
    var registerPush: () async -> Void

    static func live(client: any LoopNoticesClient) -> FeedbackFixedOptIn {
        FeedbackFixedOptIn(
            client: client,
            authorization: { await UNUserNotificationCenter.current().notificationSettings().authorizationStatus },
            requestAuthorization: { (try? await UNUserNotificationCenter.current().requestAuthorization(options: [.alert, .sound, .badge])) ?? false },
            registerPush: {
                #if os(macOS)
                await MacPushHost.shared.refresh()
                #else
                await PushHost.shared.refresh()
                #endif
            }
        )
    }

    /// 通知が届くか。未決定ならここで 1 回だけ聞く。
    func requestPermissionIfNeeded() async -> Bool {
        let allowed: Bool
        switch await authorization() {
        case .notDetermined: allowed = await requestAuthorization()
        case .denied: allowed = false
        default: allowed = true
        }
        if allowed { await registerPush() }
        return allowed
    }

    /// 送った後に呼ぶ。保存できたら true。
    func saveConsent() async -> Bool {
        do {
            try await client.setFeedbackFixedConsent(true)
            return true
        } catch {
            print("[Metamon] notices: feedback-fixed consent not saved")
            return false
        }
    }
}
#endif
