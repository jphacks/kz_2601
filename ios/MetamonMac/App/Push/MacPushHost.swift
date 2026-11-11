// 審査用ビルド(APP_STORE_REVIEW)ではプッシュごと外す(iOS の #269 と同じ・#982)。
#if !APP_STORE_REVIEW
import SwiftUI
import AppKit
import UserNotifications
import MetamonDSL
import MetamonServices

/// Mac の APNs の登録の窓口(#982)。iOS の `PushHost` の Mac 版で、トークンの送り方・送り直し・消し方は
/// 共有パッケージの `PushDeviceRegistrar`(`PUT /v1/me/devices`)をそのまま使う。違いは OS とのやり取りだけ
/// (`NSApplication.registerForRemoteNotifications`・タップは `MacNotificationRouter`)。
/// 許可済みの時だけトークンを求める(確認のダイアログはここでは出さない・iOS と同じ)。
@MainActor
final class MacPushHost {
    static let shared = MacPushHost()

    private var registrar: PushDeviceRegistrar?
    private var client: (any PushDeviceClient)?
    private var members: (any MembershipClient)?
    private var consent = NotifyConsentStore()
    private var uid: () -> String? = { nil }
    private var authorization: () async -> UNAuthorizationStatus = MacPushHost.systemAuthorization
    private var requestToken: @MainActor () -> Void = MacPushHost.systemRequestToken
    private var openApp: @MainActor (String) -> Void = MacPushHost.routeToApp

    static func systemAuthorization() async -> UNAuthorizationStatus {
        await UNUserNotificationCenter.current().notificationSettings().authorizationStatus
    }

    static func systemRequestToken() { NSApplication.shared.registerForRemoteNotifications() }

    /// 押されたプッシュのアプリを一覧で選ぶ(ウインドウがまだ無い時は出来てから・ローカル通知と同じ道)
    static func routeToApp(_ appId: String) { MacNotificationRouter.shared.open(appId: appId) }

    /// サーバーの OS の欄(`os`・32 文字まで)。iOS の "iOS 26.0" に揃える
    static func osName(_ v: OperatingSystemVersion = ProcessInfo.processInfo.operatingSystemVersion) -> String {
        v.patchVersion == 0 ? "macOS \(v.majorVersion).\(v.minorVersion)" : "macOS \(v.majorVersion).\(v.minorVersion).\(v.patchVersion)"
    }

    /// 起動時に 1 回。モック(`pushDevices` が nil)では何もしない。
    func install(services: MacServices) {
        guard let client = services.pushDevices else { return }
        install(client: client, members: services.members, defaults: .standard, environment: .current,
                uid: { [auth = services.auth] in auth.currentUser?.uid })
    }

    /// テスト用: サーバーと OS とのやり取りを差し替える。
    /// backend の `platform` は ios / android だけ(#918)なので送らない(= ios = APNs)。Mac も同じ Bundle ID = 同じトピックで届く。
    func install(client: (any PushDeviceClient)?, members: (any MembershipClient)? = nil, defaults: UserDefaults,
                 environment: PushEnvironment, appVersion: String = ClientInfo.current.version,
                 uid: @escaping () -> String?,
                 authorization: @escaping () async -> UNAuthorizationStatus = MacPushHost.systemAuthorization,
                 requestToken: @escaping @MainActor () -> Void = MacPushHost.systemRequestToken,
                 openApp: @escaping @MainActor (String) -> Void = MacPushHost.routeToApp) {
        self.client = client
        self.members = members
        consent = NotifyConsentStore(defaults: defaults)
        registrar = client.map { PushDeviceRegistrar(client: $0, defaults: defaults, environment: environment, appVersion: appVersion, os: Self.osName()) }
        self.uid = uid
        self.authorization = authorization
        self.requestToken = requestToken
        self.openApp = openApp
    }

    /// サインイン・前面復帰で呼ぶ。
    func refresh() async {
        guard let registrar, uid() != nil else { return }
        switch await authorization() {
        case .authorized, .provisional: requestToken()
        case .denied: await registrar.authorizationRevoked()
        default: break // 未決定は聞かない(通知の許可は DSL の通知を使う時に聞く・#965)
        }
    }

    func receive(token: Data) async {
        guard let registrar, let uid = uid() else { return }
        await registrar.register(token: PushPayload.tokenHex(token), uid: uid)
    }

    /// サインアウト・退会・アカウントの切り替えの前。送れなくても戻る
    func unregister() async { await registrar?.unregister() }

    /// 通知のタップ。サインアウト中は開かない。
    func open(appId: String) async {
        guard uid() != nil else { return }
        openApp(appId)
    }

    // MARK: - 他の人の行動で届く通知の同意(#339・iOS の `AppRuntime+Push` と同じ)

    /// 同意の控え。保存が無い = 受け取らない
    func consentAllowed(appId: String) -> Bool {
        guard let uid = uid() else { return false }
        return consent.allowed(uid: uid, appId: appId)
    }

    /// 切り替えを出すか: 共有の記録があり、作成者でない(作成者には同意なしで届く)。役割を確かめられない時は出す
    func canSetConsent(appId: String, dsl: AppDSL) async -> Bool {
        guard client != nil, dsl.entities.contains(where: \.isShared) else { return false }
        guard let members else { return true }
        return (try? await members.members(appId: appId))?.role != .owner
    }

    /// 同意を送る。成功したら控えも変える
    @discardableResult
    func setConsent(_ allowed: Bool, appId: String) async -> Bool {
        guard let client, let uid = uid() else { return false }
        do {
            try await client.setNotifyConsent(appId: appId, allowed: allowed)
            consent.set(allowed, uid: uid, appId: appId)
            return true
        } catch {
            print("[Metamon] push: notify consent not saved appId=\(appId) allowed=\(allowed)")
            return false
        }
    }
}

/// `registerForRemoteNotifications` の結果を受ける(⌘Q の確認の `MacAppDelegate` に足す)。
/// entitlement の無い署名(CI・未署名)では失敗するだけで、利用者には何も出さない。
extension MacAppDelegate {
    func application(_ application: NSApplication, didRegisterForRemoteNotificationsWithDeviceToken deviceToken: Data) {
        Task { await MacPushHost.shared.receive(token: deviceToken) }
    }

    func application(_ application: NSApplication, didFailToRegisterForRemoteNotificationsWithError error: Error) {
        print("[Metamon] push: registerForRemoteNotifications failed")
    }
}

/// サインイン(uid の変化)と前面復帰で登録を確かめる(iOS の `PushRegistrationObserver`)。
struct MacPushRegistrationObserver: ViewModifier {
    private let session = MacAccount.shared.session

    func body(content: Content) -> some View {
        content
            .onChange(of: session.user?.uid, initial: true) { Task { await MacPushHost.shared.refresh() } }
            .onReceive(NotificationCenter.default.publisher(for: NSApplication.didBecomeActiveNotification)) { _ in
                Task { await MacPushHost.shared.refresh() }
            }
    }
}

/// アプリの設定の「他の人の行動で届く通知」(#339・iOS の `PushConsentSection`)。楽観的 UI(先に反映し、失敗したら戻して知らせる)。
struct MacPushConsentSection: View {
    let appId: String
    let dsl: AppDSL
    @State private var available = false
    @State private var isOn = false
    @State private var failed = false

    private var host: MacPushHost { .shared }

    var body: some View {
        Group {
            if available {
                Section {
                    Toggle("このアプリの通知を受け取る", isOn: Binding(get: { isOn }, set: change))
                        .accessibilityIdentifier("appSettings.pushConsent")
                } header: {
                    Text("他の人の行動で届く通知")
                } footer: {
                    Group {
                        if failed {
                            Text("変更できませんでした。元に戻しました")
                        } else {
                            Text("他の人の書き込みなどで届く通知です。オフにすると、このアプリからは届きません。")
                        }
                    }
                    .macSecondaryText()
                }
            }
        }
        .task(id: appId) {
            isOn = host.consentAllowed(appId: appId)
            available = await host.canSetConsent(appId: appId, dsl: dsl)
        }
    }

    private func change(_ new: Bool) {
        isOn = new
        failed = false
        Task {
            guard await !host.setConsent(new, appId: appId) else { return }
            isOn = !new
            failed = true
        }
    }
}
#endif
