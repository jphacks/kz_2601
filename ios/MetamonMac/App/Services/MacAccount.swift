import SwiftUI
import AuthenticationServices
import MetamonServices

/// Mac のログイン(#930)。状態は iOS と同じ `AuthSession`(Features/Auth/AuthSession.swift を共有)。
/// Mac にはサインインの画面を置かない: Firebase につながっていて未ログインなら、起動時に匿名でログインする
/// (今までの Mac と同じく、開いてすぐ使える)。Apple でのサインインは設定の「アカウント」と、エラーの状態カードから(#1280)。
@MainActor
final class MacAccount {
    static let shared = MacAccount(services: .shared)

    let session: AuthSession
    let isLive: Bool

    init(services: MacServices) {
        session = AuthSession(service: services.auth)
        isLive = services.isLive
    }

    /// 起動時に 1 回。状態の変化を追い、未ログインなら匿名でログインする
    func start() async {
        if isLive, session.user == nil { await session.signInAnonymously() }
        await session.observe()
    }

    /// Sign in with Apple の結果を Firebase に渡す。匿名なら引き継ぐ(uid はそのまま)。
    /// その Apple ID が既に別のアカウントにあれば、そのアカウントに切り替える(Mac は作ったアプリを端末に持つので、サーバーの分は消えない)。
    /// `relogin`: セッション切れの後。引き継がずにサインインし直す
    /// - Returns: サインインできたか
    @discardableResult
    func completeApple(_ result: Result<ASAuthorization, Error>, nonce: String, relogin: Bool = false) async -> Bool {
        guard !relogin, session.user?.isAnonymous == true else {
            await session.completeAppleSignIn(result, nonce: nonce)
            return session.errorMessage == nil && session.user?.isAnonymous == false
        }
        if await session.completeAppleLink(result, nonce: nonce) { return true }
        guard session.linkConflict else { return false }
        session.linkConflict = false
        return await session.switchToExistingAppleAccount()
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
        }
    }
}
