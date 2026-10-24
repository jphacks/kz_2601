import AuthenticationServices
import UIKit
import MetamonServices

extension AuthSession {
    /// アカウント削除(#119・App Store 5.1.1(v))。匿名ユーザーも可。
    /// 1. Apple でサインインしたユーザーは Apple に再認証してもらい、そのコードでトークンを取り消す(失敗しても続行)
    /// 2. `DELETE /v1/me`(サーバーの全データと認証ユーザー)。失敗したらここで止める(端末側は消さない=やり直せる)
    /// 3. 端末のデータを消してサインアウト → サインイン画面へ
    /// - Returns: 完了したら true。Apple の再認証をキャンセルしたら false(何も消していない)。
    func deleteAccount(account: any AccountClient, runtime: AppRuntime) async throws -> Bool {
        if let user, !user.isAnonymous, service is FirebaseAuthService {
            guard let code = try await AppleReauth.authorizationCode() else { return false }
            do {
                try await service.revokeAppleToken(authorizationCode: code)
            } catch {
                // Firebase の Apple プロバイダ設定(OAuth コードフロー)が無いと失敗する。削除自体は続ける
                print("[Metamon] Apple token revoke failed (continuing): \(error)")
            }
        }
        try await account.deleteAccount()
        await runtime.wipeLocalData()
        signOut() // サーバー側でユーザーは消えているが、端末のセッションはここで破棄する
        return true
    }
}

/// アカウント削除の直前に Apple の再認証シートを出し、`authorizationCode`(5 分有効・1 回限り)を受け取る。
@MainActor
private final class AppleReauth: NSObject, ASAuthorizationControllerDelegate, ASAuthorizationControllerPresentationContextProviding {
    private var continuation: CheckedContinuation<String?, Error>?

    /// キャンセルなら nil。
    static func authorizationCode() async throws -> String? {
        let reauth = AppleReauth()
        return try await withCheckedThrowingContinuation { cont in
            reauth.continuation = cont
            let request = ASAuthorizationAppleIDProvider().createRequest()
            request.requestedScopes = [] // 再認証なので名前・メールは要らない
            let controller = ASAuthorizationController(authorizationRequests: [request])
            controller.delegate = reauth
            controller.presentationContextProvider = reauth
            controller.performRequests()
            objc_setAssociatedObject(controller, &AppleReauth.key, reauth, .OBJC_ASSOCIATION_RETAIN) // delegate は weak
        }
    }

    private static var key: UInt8 = 0

    func authorizationController(controller: ASAuthorizationController, didCompleteWithAuthorization authorization: ASAuthorization) {
        let credential = authorization.credential as? ASAuthorizationAppleIDCredential
        guard let data = credential?.authorizationCode, let code = String(data: data, encoding: .utf8) else {
            finish(.failure(AuthError.invalidAppleCredential))
            return
        }
        finish(.success(code))
    }

    func authorizationController(controller: ASAuthorizationController, didCompleteWithError error: Error) {
        if (error as? ASAuthorizationError)?.code == .canceled {
            finish(.success(nil))
        } else {
            finish(.failure(error))
        }
    }

    func presentationAnchor(for controller: ASAuthorizationController) -> ASPresentationAnchor {
        let scenes = UIApplication.shared.connectedScenes.compactMap { $0 as? UIWindowScene }
        return scenes.flatMap(\.windows).first(where: \.isKeyWindow) ?? ASPresentationAnchor()
    }

    private func finish(_ result: Result<String?, Error>) {
        continuation?.resume(with: result)
        continuation = nil
    }
}
