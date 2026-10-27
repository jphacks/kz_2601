import Foundation

/// 固定 uid の認証モック(`AppEnvironment.mock` / Preview / テスト用)。
/// `signedIn: false` で起動するとサインイン画面の動線を Firebase 無しで確認できる。
public final class MockAuthService: AuthService, @unchecked Sendable {
    public static let fixedUID = "mock00-fixed-uid"
    /// 引き継ぎ(#276)で「既に別のアカウントにある」Apple ID の既存 uid。
    public static let existingAppleUID = "mock00-apple-uid"

    /// true なら `linkWithApple` が `credentialAlreadyInUse` で失敗する(既に別のアカウントにある Apple ID を再現)。
    public var appleIDInUse = false
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
        return set(AuthUser(uid: Self.fixedUID, isAnonymous: false, displayName: name?.isEmpty == false ? name : nil))
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
        guard current.isAnonymous else { throw AuthError.alreadyLinked }
        if lock.withLock({ appleIDInUse }) {
            lock.withLock { hasPending = true }
            throw AuthError.credentialAlreadyInUse
        }
        return set(AuthUser(uid: current.uid, isAnonymous: false, displayName: current.displayName))
    }

    public func switchToPendingAppleAccount() async throws -> AuthUser {
        let pending = lock.withLock { () -> Bool in
            defer { hasPending = false }
            return hasPending
        }
        guard pending else { throw AuthError.noPendingAppleAccount }
        return set(AuthUser(uid: Self.existingAppleUID, isAnonymous: false))
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
