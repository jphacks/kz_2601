import Foundation

/// サインイン済みユーザー(Firebase `User` の薄い写し)。uid は全 API のオーナーキー。
public struct AuthUser: Sendable, Equatable {
    public var uid: String
    public var isAnonymous: Bool
    public var displayName: String?

    public init(uid: String, isAnonymous: Bool, displayName: String? = nil) {
        self.uid = uid
        self.isAnonymous = isAnonymous
        self.displayName = displayName
    }
}

public enum AuthError: LocalizedError, Sendable {
    /// Firebase 未初期化(GoogleService-Info.plist がバンドルに無い)
    case notConfigured
    case invalidAppleCredential
    /// Firebase のユーザー削除に直近のサインインが必要(`AuthErrorCode.requiresRecentLogin`)
    case requiresRecentLogin
    /// 匿名 → Apple の引き継ぎ(#276): その Apple ID は既に別のアカウントで使われている(`AuthErrorCode.credentialAlreadyInUse`)
    case credentialAlreadyInUse
    /// 引き継ぎ(#276): 既に Apple でサインイン済み(匿名ではない)
    case alreadyLinked
    /// `switchToPendingAppleAccount()` を呼んだが、切り替え先の資格情報が残っていない
    case noPendingAppleAccount

    public var errorDescription: String? {
        switch self {
        case .notConfigured: return "Firebase が設定されていません(GoogleService-Info.plist を同期してください)"
        case .invalidAppleCredential: return "Apple から ID トークンを取得できませんでした"
        case .requiresRecentLogin: return "もう一度サインインしてから削除してください"
        case .credentialAlreadyInUse: return "この Apple ID は、すでに別のアカウントで使われています"
        case .alreadyLinked: return "すでに Apple でサインインしています"
        case .noPendingAppleAccount: return "もう一度 Apple でサインインしてください"
        }
    }
}

/// 認証(G5)。実装: `FirebaseAuthService`(Live) / `MockAuthService`(Mock)。
/// Sign in with Apple の UI(nonce 生成・`ASAuthorization`)はアプリ本体が担い、ここには Apple の idToken と生 nonce だけ渡す。
public protocol AuthService: AnyObject, Sendable {
    var currentUser: AuthUser? { get }
    /// サインイン状態の変化。購読直後に現在値を 1 回流す。
    func stateChanges() -> AsyncStream<AuthUser?>
    func signInWithApple(idToken: String, nonce: String, fullName: PersonNameComponents?) async throws -> AuthUser
    func signInAnonymously() async throws -> AuthUser
    func signOut() throws
    /// Bearer に載せる Firebase ID トークン。未サインインなら nil。
    func idToken(forceRefresh: Bool) async throws -> String?
    /// Sign in with Apple のトークンを Apple 側で取り消す(アカウント削除時・#119)。`authorizationCode` は直前の Apple 認証で得た値。
    func revokeAppleToken(authorizationCode: String) async throws
    /// 現在のユーザーを Firebase Auth から削除する(アカウント削除・#119)。同じ Apple ID で次にサインインすると新しい uid になる。
    /// 直近のサインインが必要な時は `AuthError.requiresRecentLogin`。
    func deleteCurrentUser() async throws
    /// Apple の idToken で現在のユーザーを再認証する(`deleteCurrentUser` の前・requiresRecentLogin の回避)。
    func reauthenticateWithApple(idToken: String, nonce: String) async throws
    /// 匿名ユーザーに Apple ID を結びつける(#276・Firebase の credential link)。uid は変わらないので、作ったアプリとデータはそのまま。
    /// その Apple ID が既に別のアカウントにあれば `AuthError.credentialAlreadyInUse`(その時の資格情報はサービスが持ち、
    /// `switchToPendingAppleAccount()` で使う)。匿名でなければ `AuthError.alreadyLinked`。
    func linkWithApple(idToken: String, nonce: String, fullName: PersonNameComponents?) async throws -> AuthUser
    /// `credentialAlreadyInUse` の後、その Apple ID の既存アカウントに切り替える。匿名で作ったアプリは引き継がれない(uid が変わる)。
    func switchToPendingAppleAccount() async throws -> AuthUser
}

public extension AuthService {
    /// 既定は何もしない(モック)。
    func revokeAppleToken(authorizationCode: String) async throws {}
    /// 既定はサインアウト(モック)。
    func deleteCurrentUser() async throws { try signOut() }
    func reauthenticateWithApple(idToken: String, nonce: String) async throws {}
    /// 既定は未対応(テスト用の差し替え実装向け)。
    func linkWithApple(idToken: String, nonce: String, fullName: PersonNameComponents?) async throws -> AuthUser {
        throw AuthError.notConfigured
    }
    func switchToPendingAppleAccount() async throws -> AuthUser { throw AuthError.noPendingAppleAccount }
}
