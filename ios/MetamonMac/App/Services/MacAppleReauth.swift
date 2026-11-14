import AppKit
import AuthenticationServices
import MetamonServices

/// アカウント削除の直前に Apple の再認証シートを出す(#972)。iOS の `AppleReauth`(AccountDeletion.swift)の Mac 版で、
/// 違いはシートを出すウインドウ(NSWindow)だけ。`code`(5 分有効・1 回限り)は Apple のトークン取り消しに、
/// `idToken` + 生の `nonce` は Firebase の再認証に使う。
@MainActor
final class MacAppleReauth: NSObject, ASAuthorizationControllerDelegate, ASAuthorizationControllerPresentationContextProviding {
    struct Credential {
        var code: String
        var idToken: String
        var nonce: String
    }

    private var continuation: CheckedContinuation<Credential?, Error>?
    private var nonce = ""

    /// キャンセルなら nil。
    static func credential() async throws -> Credential? {
        let reauth = MacAppleReauth()
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
            objc_setAssociatedObject(controller, &MacAppleReauth.key, reauth, .OBJC_ASSOCIATION_RETAIN) // delegate は weak
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
        NSApp.keyWindow ?? NSApp.mainWindow ?? NSApp.windows.first ?? ASPresentationAnchor()
    }

    private func finish(_ result: Result<Credential?, Error>) {
        continuation?.resume(with: result)
        continuation = nil
    }
}
