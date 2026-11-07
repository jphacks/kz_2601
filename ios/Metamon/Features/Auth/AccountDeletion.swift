import AuthenticationServices
import UIKit
import MetamonServices

extension AuthSession {
    /// アカウント削除(#119・App Store 5.1.1(v))。匿名ユーザーも可。
    /// 0. Apple でサインインしたユーザーは Apple に再認証してもらい、Firebase を再認証(requiresRecentLogin の回避)して
    ///    そのコードで Apple のトークンを取り消す(どちらも失敗しても続行)
    /// 1〜4. `AccountDeletionFlow`: サーバーで論理削除 → Firebase のユーザー削除 → 端末のデータ削除 → サインアウト
    /// - Returns: 完了したら true。生成中・Apple の再認証をキャンセルしたら false(何も消していない)。
    func deleteAccount(account: any AccountClient, runtime: AppRuntime) async throws -> Bool {
        guard runtime.canDelete else { return false } // 生成中は削除しない(何も消していない)
        let isApple = user.map { !$0.isAnonymous } == true && service is FirebaseAuthService
        if isApple {
            guard let apple = try await AppleReauth.credential() else { return false }
            // 失効の仕組みと人の作業(鍵の登録)は docs/legal/apple-token-revocation.md(#246)
            _ = await AppleTokenRevocation.run(auth: service, authorizationCode: apple.code, idToken: apple.idToken, nonce: apple.nonce)
        }
        await runtime.settlePendingRecordDeletes() // 「元に戻す」を閉じる。削除に失敗しても消した記録は戻らない。成功すれば wipe で列ごと消える
        await runtime.unregisterPushDevice() // `DELETE /v1/me` の後は送れない。送れなくても続ける(#269)
        try await AccountDeletionFlow.run(
            account: account,
            auth: service,
            reauthenticate: { [service] in
                guard isApple, let apple = try await AppleReauth.credential() else { return false }
                try await service.reauthenticateWithApple(idToken: apple.idToken, nonce: apple.nonce)
                return true
            },
            wipeLocal: { await runtime.wipeLocalData() }
        )
        signOut() // セッションの表示を即反映(サインアウト済みでも安全)
        farewell = true // #102
        return true
    }
}

/// アカウント削除の直前に Apple の再認証シートを出す。`code`(5 分有効・1 回限り)は Apple のトークン取り消しに、
/// `idToken` + 生の `nonce` は Firebase の再認証に使う。
@MainActor
private final class AppleReauth: NSObject, ASAuthorizationControllerDelegate, ASAuthorizationControllerPresentationContextProviding {
    struct Credential {
        var code: String
        var idToken: String
        var nonce: String
    }

    private var continuation: CheckedContinuation<Credential?, Error>?
    private var nonce = ""

    /// キャンセルなら nil。
    static func credential() async throws -> Credential? {
        let reauth = AppleReauth()
        return try await withCheckedThrowingContinuation { cont in
            reauth.continuation = cont
            reauth.nonce = AppleNonce.random()
            let request = ASAuthorizationAppleIDProvider().createRequest()
            request.requestedScopes = [] // 再認証なので名前・メールは要らない
            request.nonce = AppleNonce.sha256(reauth.nonce)
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
        guard let codeData = credential?.authorizationCode, let code = String(data: codeData, encoding: .utf8),
              let tokenData = credential?.identityToken, let idToken = String(data: tokenData, encoding: .utf8) else {
            finish(.failure(AuthError.invalidAppleCredential))
            return
        }
        finish(.success(Credential(code: code, idToken: idToken, nonce: nonce)))
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

    private func finish(_ result: Result<Credential?, Error>) {
        continuation?.resume(with: result)
        continuation = nil
    }
}
