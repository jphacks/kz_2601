import Foundation

/// 固定 uid の認証モック(`AppEnvironment.mock` / Preview / テスト用)。
/// `signedIn: false` で起動するとサインイン画面の動線を Firebase 無しで確認できる。
public final class MockAuthService: AuthService, @unchecked Sendable {
    public static let fixedUID = "mock00-fixed-uid"
    /// 引き継ぎ(#276)で「既に別のアカウントにある」Apple ID の既存 uid。
    public static let existingAppleUID = "mock00-apple-uid"

    /// true なら `linkWithApple` が `credentialAlreadyInUse` で失敗する(既に別のアカウントにある Apple ID を再現)。
    public var appleIDInUse = false
    /// true なら `linkWithApple` が `appleEmailInUse` で失敗する(Apple ID のメールが別のアカウントにある・#361)。
    public var appleEmailInUse = false
    private var hasPending = false

    private let lock = NSLock()
    private var user: AuthUser?
    private var continuations: [UUID: AsyncStream<AuthUser?>.Continuation] = [:]

    public init(signedIn: Bool = true) {
        user = signedIn ? AuthUser(uid: Self.fixedUID, isAnonymous: true) : nil
    }

    public var currentUser: AuthUser? { lock.withLock { user } }

    public func stateChanges() -> AsyncStream<AuthUser?> {
        AsyncStream { continuation in
            let id = UUID()
            lock.withLock {
                continuations[id] = continuation
                continuation.yield(user)
            }
            continuation.onTermination = { [weak self] _ in
                self?.lock.withLock { _ = self?.continuations.removeValue(forKey: id) }
            }
        }
    }

    public func signInWithApple(idToken: String, nonce: String, fullName: PersonNameComponents?) async throws -> AuthUser {
        let name = fullName.map { PersonNameComponentsFormatter.localizedString(from: $0, style: .default) }
        return set(AuthUser(uid: Self.fixedUID, isAnonymous: false, displayName: name?.isEmpty == false ? name : nil, providers: [.apple]))
    }

    /// Apple 以外(Android の Google など)で作ったアカウントでサインインした状態にする(#361 のテスト・Preview 用)。
    @discardableResult
    public func signIn(providers: [SignInProvider], uid: String = fixedUID) -> AuthUser {
        set(AuthUser(uid: uid, isAnonymous: false, providers: Self.normalized(providers)))
    }

    public func signInAnonymously() async throws -> AuthUser {
        set(AuthUser(uid: Self.fixedUID, isAnonymous: true))
    }

    public func signOut() throws { _ = set(nil) }

    /// uid を保ったまま匿名でなくする(Firebase の link と同じ)。
    public func linkWithApple(idToken: String, nonce: String, fullName: PersonNameComponents?) async throws -> AuthUser {
        guard let current = currentUser else {
            return try await signInWithApple(idToken: idToken, nonce: nonce, fullName: fullName)
        }
        guard !current.providers.contains(.apple) else { throw AuthError.alreadyLinked }
        if lock.withLock({ appleEmailInUse }) { throw AuthError.appleEmailInUse }
        if lock.withLock({ appleIDInUse }) {
            lock.withLock { hasPending = current.isAnonymous } // 既存アカウント(#361)は切り替え先を持たない
            throw AuthError.credentialAlreadyInUse
        }
        return set(AuthUser(uid: current.uid, isAnonymous: false, displayName: current.displayName, providers: Self.normalized(current.providers + [.apple])))
    }

    /// Firebase と同じ並び(`SignInProvider.from(providerIDs:)`)にそろえる。
    private static func normalized(_ providers: [SignInProvider]) -> [SignInProvider] {
        SignInProvider.from(providerIDs: providers.map(\.rawValue))
    }

    public func switchToPendingAppleAccount() async throws -> AuthUser {
        let pending = lock.withLock { () -> Bool in
            defer { hasPending = false }
            return hasPending
        }
        guard pending else { throw AuthError.noPendingAppleAccount }
        return set(AuthUser(uid: Self.existingAppleUID, isAnonymous: false, providers: [.apple]))
    }

    /// モックはトークンを持たない(Authorization ヘッダ無しで送る)。
    public func idToken(forceRefresh: Bool) async throws -> String? { nil }

    @discardableResult
    private func set(_ u: AuthUser?) -> AuthUser! {
        lock.withLock {
            user = u
            for c in continuations.values { c.yield(u) }
        }
        return u
    }
}
