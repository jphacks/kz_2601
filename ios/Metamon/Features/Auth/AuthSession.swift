import AuthenticationServices
import CryptoKit
import Observation
import SwiftUI
import MetamonServices

/// 既存のアカウントに Apple をつないだ結果(#361)。画面は iOS の設定だけ(Mac にはつなぐ UI が無い)。
enum AppleLinkOutcome: Equatable {
    case linked
    case cancelled
    /// その Apple ID は別のアカウントで使われている(`credentialAlreadyInUse`)。何も変えない
    case inUseByOtherAccount
    /// その Apple ID のメールアドレスが別のアカウントで使われている(`appleEmailInUse`)。何も変えない
    case emailInUseByOtherAccount
    case failed(String)
}

/// サインイン状態(`AuthService` の観測可能ラッパー)。`user == nil` なら `SignInView`、それ以外は `RootView`。
@MainActor
@Observable
final class AuthSession {
    private(set) var user: AuthUser?
    private(set) var isBusy = false
    var errorMessage: String?
    /// 自分でログアウト / アカウント削除した直後か(サインイン画面でバイバイのツルを出す・#102)
    var farewell = false
    /// 匿名 → Apple の引き継ぎ(#276)で、その Apple ID が既に別のアカウントにあった。切り替えるかを確認する
    var linkConflict = false
    /// 引き継ぎの失敗理由(設定・案内シートに出す。サインイン画面の `errorMessage` とは分ける)
    var linkError: String?

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
            guard let apple = Self.appleToken(authorization) else {
                errorMessage = AuthError.invalidAppleCredential.localizedDescription
                return
            }
            await perform { try await service.signInWithApple(idToken: apple.idToken, nonce: nonce, fullName: apple.fullName) }
        }
    }

    /// 匿名 → Apple の引き継ぎ(#276)。`SignInWithAppleButton` の completion を受けて Firebase で link する(uid はそのまま)。
    /// - Returns: 引き継げたら true。キャンセル・失敗・既に別アカウント(`linkConflict` が立つ)は false。
    @discardableResult
    func completeAppleLink(_ result: Result<ASAuthorization, Error>, nonce: String) async -> Bool {
        switch result {
        case .failure(let error):
            if (error as? ASAuthorizationError)?.code != .canceled { linkError = error.localizedDescription }
            return false
        case .success(let authorization):
            guard let apple = Self.appleToken(authorization) else {
                linkError = AuthError.invalidAppleCredential.localizedDescription
                return false
            }
            return await linkApple(idToken: apple.idToken, nonce: nonce, fullName: apple.fullName)
        }
    }

    /// Apple の idToken で今の匿名ユーザーに Apple を結びつける。
    @discardableResult
    func linkApple(idToken: String, nonce: String, fullName: PersonNameComponents?) async -> Bool {
        isBusy = true
        defer { isBusy = false }
        linkError = nil
        do {
            user = try await service.linkWithApple(idToken: idToken, nonce: nonce, fullName: fullName)
            return true
        } catch AuthError.credentialAlreadyInUse {
            linkConflict = true
        } catch {
            linkError = error.localizedDescription
        }
        return false
    }

    /// `linkConflict` の後、その Apple ID の既存アカウントへ切り替える。匿名で作ったアプリはこの端末から見えなくなる。
    /// - Parameter beforeSwitch: 切り替えの前に呼ぶ(匿名の uid に付いた通知の端末の登録を消す・サインアウトと同じ #269)。
    ///   切り替えると uid が変わるので、新しい uid では前面に戻った時に登録し直す。
    /// - Parameter afterFailedSwitch: 切り替えられなかった時に呼ぶ(匿名の uid のまま残るので、消した登録を戻す)。
    @discardableResult
    func switchToExistingAppleAccount(beforeSwitch: @MainActor () async -> Void = {},
                                      afterFailedSwitch: @MainActor () async -> Void = {}) async -> Bool {
        isBusy = true
        defer { isBusy = false }
        linkError = nil
        await beforeSwitch()
        do {
            user = try await service.switchToPendingAppleAccount()
            return true
        } catch {
            linkError = error.localizedDescription
            await afterFailedSwitch()
            return false
        }
    }

    /// 既にあるアカウント(Apple 以外)に Apple をつなぐ(#361・設定の「サインインの方法」)。
    /// その Apple ID が別のアカウントにあっても、切り替えもまとめもせず `.inUseByOtherAccount` を返すだけ。
    func linkAppleToCurrentAccount(_ result: Result<ASAuthorization, Error>, nonce: String) async -> AppleLinkOutcome {
        switch result {
        case .failure(let error):
            if (error as? ASAuthorizationError)?.code == .canceled { return .cancelled }
            return Self.linkFailed(error)
        case .success(let authorization):
            guard let apple = Self.appleToken(authorization) else {
                return .failed(AuthError.invalidAppleCredential.localizedDescription)
            }
            return await linkAppleToCurrentAccount(idToken: apple.idToken, nonce: nonce, fullName: apple.fullName)
        }
    }

    func linkAppleToCurrentAccount(idToken: String, nonce: String, fullName: PersonNameComponents?) async -> AppleLinkOutcome {
        isBusy = true
        defer { isBusy = false }
        do {
            user = try await service.linkWithApple(idToken: idToken, nonce: nonce, fullName: fullName)
            return .linked
        } catch AuthError.credentialAlreadyInUse {
            return .inUseByOtherAccount
        } catch AuthError.appleEmailInUse {
            return .emailInUseByOtherAccount
        } catch AuthError.alreadyLinked {
            // 手元の providers が古い(別の端末でつないだ)。読み直して「つながっています」にする
            await refreshUser()
            return .linked
        } catch {
            return Self.linkFailed(error)
        }
    }

    /// サーバーから読み直して `user` を最新にする(設定の「サインインの方法」を開いた時・#361)。失敗しても手元の値のまま。
    func refreshUser() async {
        do {
            if let fresh = try await service.reloadCurrentUser() { user = fresh }
        } catch {
            ErrorLog.record(error, context: "auth reload")
        }
    }

    /// 訳していない失敗(Firebase・Apple の英語の文)は画面に出さず、ログにだけ残す。
    private static func linkFailed(_ error: Error) -> AppleLinkOutcome {
        if let known = error as? AuthError { return .failed(known.localizedDescription) }
        ErrorLog.record(error, context: "link Apple")
        return .failed(L10n.tr("Apple をつなげませんでした"))
    }

    private static func appleToken(_ authorization: ASAuthorization) -> (idToken: String, fullName: PersonNameComponents?)? {
        guard let credential = authorization.credential as? ASAuthorizationAppleIDCredential,
              let data = credential.identityToken,
              let idToken = String(data: data, encoding: .utf8) else { return nil }
        return (idToken, credential.fullName)
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
