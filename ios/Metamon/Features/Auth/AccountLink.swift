import AuthenticationServices
import SwiftUI
import MetamonServices

// 匿名で作ったアプリを Apple ID に引き継ぐ(#276)。Firebase の credential link なので uid は変わらず、
// appconfig のアプリとデータ(users/{uid}/...)はそのまま。機種変更・再インストール後は同じ Apple ID でサインインすれば戻れる。

/// 「引き継ぎのためにサインイン」の案内を出したか(端末ごとに 1 回だけ)。
/// 最初のアプリの直後には出さず(H7: 完成直後のお知らせを減らす)、作った起動を覚えておいて次の起動で出す。
enum AccountLinkPrompt {
    static let key = "metamon.accountLinkPrompted"
    /// 案内を待っている(値 = 最初のアプリを作った起動の id)
    static let pendingKey = "metamon.accountLinkPending"
    /// この起動の id(プロセスごと)
    static let currentLaunch = UUID().uuidString

    static func shouldOffer(_ defaults: UserDefaults = .standard) -> Bool { !defaults.bool(forKey: key) }
    static func markOffered(_ defaults: UserDefaults = .standard) {
        defaults.set(true, forKey: key)
        defaults.removeObject(forKey: pendingKey)
    }

    /// 最初のアプリを作った: この起動の id で予約する(既に予約があれば前のを残す)。
    static func markPending(_ defaults: UserDefaults = .standard, launch: String = currentLaunch) {
        guard defaults.string(forKey: pendingKey) == nil else { return }
        defaults.set(launch, forKey: pendingKey)
    }

    /// 前の起動で予約した案内があるか(同じ起動の予約はまだ出さない)。
    static func isDue(_ defaults: UserDefaults = .standard, launch: String = currentLaunch) -> Bool {
        guard let pending = defaults.string(forKey: pendingKey) else { return false }
        return pending != launch
    }
}

extension AppRuntime {
    /// 匿名ユーザーが初めてアプリを作った: 引き継ぎの案内を次の起動に予約する(H7: 最初のアプリの直後には出さない)。
    func offerAccountLinkIfNeeded(defaults: UserDefaults = .standard, launch: String = AccountLinkPrompt.currentLaunch) {
        guard isAnonymousUser?() == true, AccountLinkPrompt.shouldOffer(defaults) else { return }
        AccountLinkPrompt.markPending(defaults, launch: launch)
    }

    /// 起動時(`bootstrap`): 前の起動で予約した案内があれば 1 回だけ出す。出す順番は `PostBuildNotice`(他のお知らせの後)。
    func presentDeferredAccountLinkIfNeeded(defaults: UserDefaults = .standard, launch: String = AccountLinkPrompt.currentLaunch) {
        guard AccountLinkPrompt.isDue(defaults, launch: launch) else { return }
        guard isAnonymousUser?() == true, AccountLinkPrompt.shouldOffer(defaults) else {
            defaults.removeObject(forKey: AccountLinkPrompt.pendingKey) // もう Apple に引き継いだ等
            return
        }
        AccountLinkPrompt.markOffered(defaults)
        let delay = linkPromptDelay
        Task { [weak self] in
            try? await Task.sleep(for: delay) // 起動直後の画面が落ち着いてから
            guard let self, self.isAnonymousUser?() == true else { return }
            self.showLinkPrompt = true
        }
    }

    /// この端末のアプリがまだ空(音声画面だけ)なら、サーバーにある一番新しいアプリを開く。
    /// 再インストール後のサインイン・別アカウントへの切り替えの直後に使う。
    func resumeLatestAppIfEmpty() async {
        guard services.isLive, dsl.isVoiceOnly, morphJob == nil,
              let list = try? await services.appConfig.listApps() else { return }
        apps = list
        guard let latest = Self.latestApp(in: appDeletion.filter(list), excluding: document.appId) else { return }
        print("[Metamon] resume latest app: \(latest.id)")
        await switchApp(to: latest.id)
    }

    /// 一覧に出るアプリ(archived・未生成を除く)のうち、更新が一番新しいもの。
    nonisolated static func latestApp(in apps: [AppSummary], excluding current: String) -> AppSummary? {
        AppSummary.visible(apps, current: nil).first { $0.id != current }
    }

    /// 既存の Apple アカウントへ切り替えた後: 匿名の時の端末のデータを消し、そのアカウントのアプリを開く。
    func adoptSwitchedAccount() async {
        await wipeLocalData()
        await resumeLatestAppIfEmpty()
    }
}

/// 匿名ユーザー向けの「Apple でサインインして引き継ぐ」ボタン。既に別のアカウントにある Apple ID なら切り替えるかを確認する。
struct AppleLinkButton: View {
    var onLinked: () -> Void = {}
    @Environment(AuthSession.self) private var session
    @Environment(AppRuntime.self) private var rt
    @Environment(\.colorScheme) private var colorScheme
    @State private var nonce = ""

    var body: some View {
        @Bindable var session = session
        SignInWithAppleButton(.continue) { request in
            let raw = AppleNonce.random()
            nonce = raw
            request.requestedScopes = [.fullName]
            request.nonce = AppleNonce.sha256(raw)
        } onCompletion: { result in
            let raw = nonce
            Task {
                if await session.completeAppleLink(result, nonce: raw) {
                    rt.toast = L10n.tr("Apple に引き継ぎました。機種変更のあとも同じ Apple ID で戻れます")
                    onLinked()
                }
            }
        }
        .signInWithAppleButtonStyle(colorScheme == .dark ? .white : .black)
        .frame(height: 48)
        .clipShape(RoundedRectangle(cornerRadius: Theme.Radius.md, style: .continuous))
        .disabled(session.isBusy)
        .accessibilityIdentifier("account.linkApple")
        .alert("この Apple ID は別のアカウントで使われています", isPresented: $session.linkConflict) {
            Button("そのアカウントに切り替える", role: .destructive) {
                Task {
                    guard await session.switchToExistingAppleAccount(beforeSwitch: { await rt.unregisterPushDevice() },
                                                                     afterFailedSwitch: { await rt.registerPushDevice() }) else { return }
                    await rt.adoptSwitchedAccount()
                    onLinked()
                }
            }
            Button("キャンセル", role: .cancel) {}
        } message: {
            Text(L10n.key("切り替えると、その Apple ID で作ったアプリが開きます。いま匿名で作ったアプリは引き継がれず、この端末から見えなくなります。",
                          review: "切り替えると、その Apple ID で用意した記録帳が開きます。いま匿名で用意した記録帳は引き継がれず、この端末から見えなくなります。"))
        }
    }
}

/// 匿名で初めてアプリを作った後に 1 回だけ出す案内。
struct AccountLinkPromptSheet: View {
    @Environment(AuthSession.self) private var session
    @Environment(\.dismiss) private var dismiss

    var body: some View {
        // 横向き(#625)はシートの高さが足りず「あとで」が見切れるのでスクロールに入れる
        ScrollView {
            VStack(spacing: Theme.Spacing.lg) {
                Image(systemName: "person.crop.circle.badge.checkmark")
                    .font(.system(size: 48))
                    .foregroundStyle(Theme.brandGradient)
                VStack(spacing: Theme.Spacing.sm) {
                    Text(L10n.key("作ったアプリを残しておきませんか", review: "用意した記録帳を残しておきませんか"))
                        .font(.title3.weight(.bold))
                        .multilineTextAlignment(.center)
                    Text(L10n.key("いまは匿名で使っています。Apple でサインインしておくと、機種変更やアプリの入れ直しのあとも、作ったアプリとデータに戻れます。",
                                   review: "いまは匿名で使っています。Apple でサインインしておくと、機種変更やアプリの入れ直しのあとも、用意した記録帳とデータに戻れます。"))
                        .font(.subheadline)
                        .foregroundStyle(.secondary)
                        .multilineTextAlignment(.center)
                }
                VStack(spacing: Theme.Spacing.sm) {
                    AppleLinkButton { dismiss() }
                    if let message = session.linkError {
                        Text(message).font(.footnote).foregroundStyle(.red).multilineTextAlignment(.center)
                    }
                    Button("あとで") { dismiss() }
                        .font(.footnote.weight(.medium))
                        .foregroundStyle(.secondary)
                        .padding(.vertical, Theme.Spacing.sm)
                }
            }
            .padding(Theme.Spacing.lg)
            .readableWidth()
        }
        .scrollBounceBehavior(.basedOnSize)
        .presentationDetents([.medium])
        .presentationDragIndicator(.visible)
    }
}

/// RootView に付ける: 案内シートは他のシート(発話・同意・フォーム)と完成直後のお知らせ(`PostBuildNotice`)が閉じてから出す。
struct AccountLinkPromptHost: ViewModifier {
    @Environment(AppRuntime.self) private var rt

    func body(content: Content) -> some View {
        content.sheet(isPresented: Binding(
            get: { rt.showLinkPrompt && rt.isNoticeTurn(.accountLink) && !rt.showVoiceSheet && !rt.showAIConsent && !rt.showTutorial && !rt.showAppSwitcher && rt.sheet == nil && rt.fullScreen == nil },
            set: { if !$0 { rt.showLinkPrompt = false } }
        )) {
            AccountLinkPromptSheet()
        }
    }
}
