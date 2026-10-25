import AuthenticationServices
import CryptoKit
import Observation
import SwiftUI
import MetamonServices

/// サインイン状態(`AuthService` の観測可能ラッパー)。`user == nil` なら `SignInView`、それ以外は `RootView`。
@MainActor
@Observable
final class AuthSession {
    private(set) var user: AuthUser?
    private(set) var isBusy = false
    var errorMessage: String?
    /// 自分でログアウト / アカウント削除した直後か(サインイン画面でバイバイのツルを出す・#102)
    var farewell = false

    let service: any AuthService

    init(service: any AuthService) {
        self.service = service
        self.user = service.currentUser
    }

    /// Firebase の状態変化を追従する(別経路のサインアウト・トークン失効も反映)。
    func observe() async {
        #if DEBUG
        // ヘッドレス E2E 用: `METAMON_AUTO_ANON=1` で起動時に匿名サインイン(シミュレータの simctl launch から渡す)
        if user == nil, ProcessInfo.processInfo.environment["METAMON_AUTO_ANON"] == "1" {
            await signInAnonymously()
            print("[Metamon] auto anonymous sign-in → \(user.map { "uid=\($0.uid)" } ?? "failed: \(errorMessage ?? "?")")")
        }
        // スクショ用: `METAMON_AUTO_SIGNOUT=1` で 2 秒後にログアウト(バイバイのツル・#102)
        if ProcessInfo.processInfo.environment["METAMON_AUTO_SIGNOUT"] == "1" {
            Task { try? await Task.sleep(for: .seconds(2)); signOut() }
        }
        #endif
        for await u in service.stateChanges() {
            user = u
        }
    }

    /// `SignInWithAppleButton` の completion を受けて Firebase に交換する。`nonce` は request 時に生成した生の値。
    func completeAppleSignIn(_ result: Result<ASAuthorization, Error>, nonce: String) async {
        switch result {
        case .failure(let error):
            if (error as? ASAuthorizationError)?.code == .canceled { return }
            errorMessage = error.localizedDescription
        case .success(let authorization):
            guard let credential = authorization.credential as? ASAuthorizationAppleIDCredential,
                  let data = credential.identityToken,
                  let idToken = String(data: data, encoding: .utf8) else {
                errorMessage = AuthError.invalidAppleCredential.localizedDescription
                return
            }
            await perform { try await service.signInWithApple(idToken: idToken, nonce: nonce, fullName: credential.fullName) }
        }
    }

    func signInAnonymously() async {
        await perform { try await service.signInAnonymously() }
    }

    func signOut() {
        let wasSignedIn = user != nil
        do {
            try service.signOut()
            user = service.currentUser
            farewell = wasSignedIn
        } catch {
            errorMessage = error.localizedDescription
        }
    }

    /// セッション切れ(#60: トークン再取得後も 401)。サインアウトしてサインイン画面に理由を出す。
    func expire() {
        signOut()
        farewell = false
        errorMessage = ErrorCopy.sessionExpiredSignIn
        print("[Metamon] session expired → sign-in screen")
    }

    private func perform(_ body: () async throws -> AuthUser) async {
        isBusy = true
        defer { isBusy = false }
        errorMessage = nil
        do {
            user = try await body()
        } catch {
            errorMessage = error.localizedDescription
        }
    }
}

/// Sign in with Apple の replay 防止 nonce。生値を Firebase へ、SHA-256(hex) を Apple へ渡す。
enum AppleNonce {
    static func random(length: Int = 32) -> String {
        let charset = Array("0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz-._")
        var generator = SystemRandomNumberGenerator() // Apple プラットフォームでは暗号学的に安全
        return String((0..<length).map { _ in charset.randomElement(using: &generator)! })
    }

    static func sha256(_ input: String) -> String {
        SHA256.hash(data: Data(input.utf8)).map { String(format: "%02x", $0) }.joined()
    }
}
