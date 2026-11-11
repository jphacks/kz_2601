import SwiftUI
import AuthenticationServices
import MetamonServices

/// Mac のログイン(#930)。状態は iOS と同じ `AuthSession`(Features/Auth/AuthSession.swift を共有)。
/// Mac にはサインインの画面を置かない: Firebase につながっていて未ログインなら、起動時に匿名でログインする
/// (今までの Mac と同じく、開いてすぐ使える)。Apple でのサインインは設定の「アカウント」と、エラーの状態カードから(#1280)。
@MainActor
@Observable
final class MacAccount {
    static let shared = MacAccount(services: .shared)

    let session: AuthSession
    let isLive: Bool
    /// アカウント削除の相手(#972・`DELETE /v1/me`)
    let account: any AccountClient
    /// サインアウト・削除の後始末(#972)。一覧(`MacAppLibrary`)はアプリの起動時に渡す
    @ObservationIgnored var wipe = MacLocalWipe()
    /// 削除の前の Apple の再認証(#972)。キャンセルなら nil。テストで差し替える
    @ObservationIgnored var appleReauth: @MainActor () async throws -> MacAppleReauth.Credential? = { try await MacAppleReauth.credential() }
    /// サインアウト・削除の途中(ボタンを止める・進み具合を出す)
    private(set) var busy: Busy?

    enum Busy { case signingOut, deleting }

    init(services: MacServices) {
        session = AuthSession(service: services.auth)
        isLive = services.isLive
        account = services.account
    }

    init(session: AuthSession, account: any AccountClient, isLive: Bool) {
        self.session = session
        self.account = account
        self.isLive = isLive
    }

    /// 起動時に 1 回。状態の変化を追い、未ログインなら匿名でログインする
    func start() async {
        if isLive, session.user == nil { await session.signInAnonymously() }
        await adoptUser(session.user?.uid)
        await session.observe()
    }

    /// 端末のデータの持ち主(uid)。サインアウト・削除の wipe で消え、次のサインインで入れ直す
    static let ownerUidKey = "metamon.mac.accountUid"

    /// uid が変わった(既存の Apple アカウントへの切り替え・別の Apple ID でサインインし直し・外でサインアウトされた後)時は、
    /// 前のアカウントの端末のデータを消す(iOS の `adoptSwitchedAccount` と同じ・#972)。
    /// 引き継ぎ(匿名 → Apple)は uid が変わらないので消さない。何度呼んでも同じ uid なら何もしない
    func adoptUser(_ uid: String?) async {
        guard let uid else { return }
        let defaults = wipe.defaults
        let owner = defaults.string(forKey: Self.ownerUidKey)
        guard owner != uid else { return }
        defaults.set(uid, forKey: Self.ownerUidKey) // 先に置いて、同時に呼ばれても 2 回消さない
        guard owner != nil else { return } // 初めての記録(この版に上げた直後など)は前の持ち主が分からないので消さない
        await wipe.run()
        defaults.set(uid, forKey: Self.ownerUidKey) // wipe が UserDefaults ごと消すので入れ直す
    }

    /// Sign in with Apple の結果を Firebase に渡す。匿名なら引き継ぐ(uid はそのまま)。
    /// その Apple ID が既に別のアカウントにあれば、そのアカウントに切り替え、匿名の時の端末のデータを消す(iOS と同じ・#972)。
    /// `relogin`: セッション切れの後。引き継がずにサインインし直す
    /// - Returns: サインインできたか
    @discardableResult
    func completeApple(_ result: Result<ASAuthorization, Error>, nonce: String, relogin: Bool = false) async -> Bool {
        guard !relogin, session.user?.isAnonymous == true else {
            await session.completeAppleSignIn(result, nonce: nonce)
            await adoptUser(session.user?.uid) // 別の Apple ID で入り直したら前のデータを消す
            return session.errorMessage == nil && session.user?.isAnonymous == false
        }
        if await session.completeAppleLink(result, nonce: nonce) { return true }
        guard session.linkConflict else { return false }
        session.linkConflict = false
        // 前のアカウントの端末の登録を消してから切り替える。失敗したら今の uid で戻す(iOS の `AccountLink` と同じ・#982)
        guard await session.switchToExistingAppleAccount(beforeSwitch: { await Self.unregisterPush() },
                                                         afterFailedSwitch: { await Self.registerPush() }) else { return false }
        await adoptUser(session.user?.uid) // 匿名の時のデータを消す(iOS の `adoptSwitchedAccount`)
        return true
    }

    // MARK: - サインアウト・アカウント削除(#972・iOS の `signOutAndWipe` / `deleteAccount` と同じ順)

    /// ログアウトの確認に出す内容。開いている全部のアプリの未送信を数える
    func signOutPrompt() async -> SignOutFlow.Prompt {
        let engines = MacRecordVault.liveEngines
        for engine in engines { await engine.commitPendingDeletions() } // 取り消し待ちの削除も未送信に数える(iOS と同じ)
        let pending = engines.reduce(0) { $0 + $1.pendingCount }
        return SignOutFlow.prompt(pending: pending, isOnline: engines.allSatisfy(\.isOnline))
    }

    /// 設定の「ログアウト」。端末のデータを消してからサインアウトし、Mac はサインインの画面を持たないので新しい匿名で入り直す。
    /// - Returns: `.unsent(n)` なら送り切れず、何も消していない(サインインしたまま)
    @discardableResult
    func signOutAndWipe(choice: SignOutFlow.Choice) async throws -> SignOutFlow.Outcome {
        busy = .signingOut
        defer { busy = nil }
        let wipe = wipe
        let outcome = try await SignOutFlow.run(
            choice: choice,
            auth: session.service,
            flush: {
                var left = 0
                for engine in MacRecordVault.liveEngines { left += await engine.flush() }
                return left
            },
            wipeLocal: {
                await Self.unregisterPush() // 送れなくても続ける(iOS の `signOutAndWipe` と同じ・#982)
                await wipe.run()
            }
        )
        guard outcome == .signedOut else { return outcome }
        session.signOut() // セッションの表示を即反映(サインアウト済みでも安全)
        await startFresh()
        return outcome
    }

    /// アカウント削除(App Store 5.1.1(v))。Apple でサインインしていれば再認証してトークンを取り消し、
    /// サーバーで論理削除 → Firebase のユーザー削除 → 端末のデータ削除 → サインアウト(`AccountDeletionFlow`・iOS と同じ)。
    /// - Returns: 完了したら true。Apple の再認証をキャンセルしたら false(何も消していない)
    func deleteAccount() async throws -> Bool {
        busy = .deleting
        defer { busy = nil }
        let service = session.service
        let isApple = session.user.map { !$0.isAnonymous } == true && service is FirebaseAuthService
        if isApple {
            guard let apple = try await appleReauth() else { return false }
            // 失効の仕組みと人の作業(鍵の登録)は docs/legal/apple-token-revocation.md(#246)
            _ = await AppleTokenRevocation.run(auth: service, authorizationCode: apple.code, idToken: apple.idToken, nonce: apple.nonce)
        }
        await Self.unregisterPush() // `DELETE /v1/me` の後は送れない。送れなくても続ける(iOS と同じ・#982)
        let wipe = wipe, reauth = appleReauth
        do {
            try await AccountDeletionFlow.run(
                account: account,
                auth: service,
                reauthenticate: {
                    guard isApple, let apple = try await reauth() else { return false }
                    try await service.reauthenticateWithApple(idToken: apple.idToken, nonce: apple.nonce)
                    return true
                },
                wipeLocal: { await wipe.run() }
            )
        } catch {
            await Self.registerPush() // 消せなかった(サインインしたまま): 端末の登録を戻す(切り替えの失敗と同じ)
            throw error
        }
        session.signOut()
        await startFresh()
        return true
    }

    /// プッシュの端末の登録(#982)。審査用ビルドはプッシュごと外す
    private static func unregisterPush() async {
        #if !APP_STORE_REVIEW
        await MacPushHost.shared.unregister()
        #endif
    }

    private static func registerPush() async {
        #if !APP_STORE_REVIEW
        await MacPushHost.shared.refresh()
        #endif
    }

    /// サインアウトの後: Firebase につながっていれば新しい匿名で入る(起動時と同じ・開いてすぐ使える)
    private func startFresh() async {
        session.farewell = false
        if isLive, session.user == nil { await session.signInAnonymously() }
        await adoptUser(session.user?.uid) // wipe 済みなので持ち主を入れるだけ
    }

    /// 失敗の一言(設定・カードの下に出す)。サインインし直しと引き継ぎで置き場が分かれているのをまとめる
    var errorMessage: String? { session.linkError ?? session.errorMessage }
}

/// Mac の「Apple でサインイン」ボタン(#930)。設定の「アカウント」とエラーの状態カード(#1280)で使う
struct MacAppleSignInButton: View {
    var relogin = false
    var onSignedIn: () -> Void = {}
    @Environment(\.colorScheme) private var colorScheme
    @State private var nonce = ""

    var body: some View {
        SignInWithAppleButton(.signIn) { request in
            let raw = AppleNonce.random()
            nonce = raw
            request.requestedScopes = [.fullName, .email]
            request.nonce = AppleNonce.sha256(raw)
        } onCompletion: { result in
            let raw = nonce
            Task {
                if await MacAccount.shared.completeApple(result, nonce: raw, relogin: relogin) { onSignedIn() }
            }
        }
        .signInWithAppleButtonStyle(colorScheme == .dark ? .white : .black)
        .frame(width: 200, height: 28)
        .disabled(MacAccount.shared.session.isBusy)
        .accessibilityIdentifier("account.signInApple")
    }
}

/// クラッシュ報告に付ける本人の区別は uid だけ(名前・内容は付けない・#296)。iOS の `MetamonApp` と同じ
struct MacCrashReportUser: ViewModifier {
    private let session = MacAccount.shared.session

    func body(content: Content) -> some View {
        content.onChange(of: session.user?.uid, initial: true) { _, uid in
            CrashReportingPreference.shared.updateUser(uid: uid)
            Task { await MacAccount.shared.adoptUser(uid) } // どの経路で uid が変わっても前のデータを残さない(#972)
        }
    }
}
