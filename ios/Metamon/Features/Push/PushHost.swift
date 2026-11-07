// 審査用ビルド(APP_STORE_REVIEW)ではプッシュごと外す(#269・docs/ios/review-build.md)。
#if !APP_STORE_REVIEW
import SwiftUI
import UserNotifications
import MetamonServices

/// APNs の登録の窓口。許可済みの時だけトークンを求める(確認のダイアログはここでは出さない)。
@MainActor
final class PushHost {
    static let shared = PushHost()

    private var registrar: PushDeviceRegistrar?
    private var uid: () -> String? = { nil }
    private var authorization: () async -> UNAuthorizationStatus = { await UNUserNotificationCenter.current().notificationSettings().authorizationStatus }
    private var requestToken: () -> Void = { UIApplication.shared.registerForRemoteNotifications() }
    private var openApp: (String) async -> Void = { await IntentHost.shared.request(.openFromNotification(appId: $0)) }

    /// モック(`pushDevices` が nil)では何もしない。
    func install(services: Services) {
        guard let client = services.pushDevices else { return }
        let info = ClientInfo.current
        registrar = PushDeviceRegistrar(client: client, environment: .current, appVersion: info.version,
                                        os: "iOS \(UIDevice.current.systemVersion)")
        uid = { [auth = services.auth] in auth.currentUser?.uid }
    }

    /// テスト用: OS とのやり取りを差し替える。
    func install(registrar: PushDeviceRegistrar?, uid: @escaping () -> String?, authorization: @escaping () async -> UNAuthorizationStatus,
                 requestToken: @escaping () -> Void, openApp: @escaping (String) async -> Void) {
        self.registrar = registrar
        self.uid = uid
        self.authorization = authorization
        self.requestToken = requestToken
        self.openApp = openApp
    }

    /// サインイン・前面復帰で呼ぶ(許可のダイアログの後も前面に戻るのでここを通る)。
    func refresh() async {
        guard let registrar, uid() != nil else { return }
        switch await authorization() {
        case .authorized, .provisional, .ephemeral: requestToken()
        case .denied: await registrar.authorizationRevoked()
        default: break // 未決定は聞かない。許可の流れは今までどおり
        }
    }

    func receive(token: Data) async {
        guard let registrar, let uid = uid() else { return }
        await registrar.register(token: PushPayload.tokenHex(token), uid: uid)
    }

    func unregister() async { await registrar?.unregister() }

    /// 通知のタップ。サインアウト中は開かない。
    func open(appId: String) async {
        guard uid() != nil else { return }
        await openApp(appId)
    }
}

/// `registerForRemoteNotifications` の結果を受ける。aps-environment が無い間は失敗するだけで、利用者には何も出さない。
final class PushAppDelegate: NSObject, UIApplicationDelegate {
    func application(_ application: UIApplication, didRegisterForRemoteNotificationsWithDeviceToken deviceToken: Data) {
        Task { await PushHost.shared.receive(token: deviceToken) }
    }

    func application(_ application: UIApplication, didFailToRegisterForRemoteNotificationsWithError error: Error) {
        print("[Metamon] push: registerForRemoteNotifications failed")
    }
}

extension ForegroundNotificationPresenter {
    /// プッシュのタップで該当アプリを開く。ローカル通知には appId が無いので今までどおり何もしない。
    func userNotificationCenter(_ center: UNUserNotificationCenter, didReceive response: UNNotificationResponse) async {
        guard response.actionIdentifier == UNNotificationDefaultActionIdentifier,
              let appId = PushPayload.appId(from: response.notification.request.content.userInfo) else { return }
        await PushHost.shared.open(appId: appId)
    }
}

/// サインイン(uid の変化)と前面復帰で登録を確かめる。
struct PushRegistrationObserver: ViewModifier {
    let uid: String?
    @Environment(\.scenePhase) private var scenePhase

    func body(content: Content) -> some View {
        content
            .onChange(of: uid, initial: true) { Task { await PushHost.shared.refresh() } }
            .onChange(of: scenePhase) { _, phase in
                if phase == .active { Task { await PushHost.shared.refresh() } }
            }
    }
}
#endif
