import Foundation
import MetamonServices

extension AuthSession {
    /// 設定の「ログアウト」。アカウント削除と同じ後片付け(`wipeLocalData`)で端末のデータを消してからサインアウトする。
    /// - Returns: `.unsent(n)` なら送り切れず、何も消していない(サインインしたまま)
    @discardableResult
    func signOutAndWipe(runtime: AppRuntime, choice: SignOutFlow.Choice) async throws -> SignOutFlow.Outcome {
        let wasSignedIn = user != nil
        let outcome = try await SignOutFlow.run(
            choice: choice,
            auth: service,
            flush: { await runtime.recordSync.flush() },
            wipeLocal: { await runtime.wipeLocalData() }
        )
        guard outcome == .signedOut else { return outcome }
        signOut() // セッションの表示を即反映(サインアウト済みでも安全)
        farewell = wasSignedIn // #102
        return outcome
    }
}

/// ログアウトの確認の文言。
enum SignOutCopy {
    static func message(isAnonymous: Bool, prompt: SignOutFlow.Prompt) -> String {
        var lines = [isAnonymous ? anonymous : apple]
        if case .unsent(let n, let canSend) = prompt {
            lines.append(canSend ? L10n.tr("まだ送っていない記録が \(n) 件あります。送ってからログアウトします。")
                                 : L10n.tr("まだ送っていない記録が \(n) 件あります。今はオフラインのため送れません。送らずにログアウトすると、この記録は消えます。"))
        }
        return lines.joined(separator: "\n\n")
    }

    /// 匿名はサインアウトすると二度と戻れない
    static var anonymous: String {
        L10n.pick(L10n.tr("この端末のデータが消え、匿名のアカウントには戻れません。作ったアプリと記録も開けなくなります。残したい時は、先に「Apple でサインイン」で引き継いでください。"),
                  review: L10n.tr("この端末のデータが消え、匿名のアカウントには戻れません。用意した記録帳と記録も開けなくなります。残したい時は、先に「Apple でサインイン」で引き継いでください。"))
    }

    static var apple: String {
        L10n.pick(L10n.tr("この端末のデータは消えます。もう一度サインインすると、作ったアプリと送り済みの記録は戻ります。"),
                  review: L10n.tr("この端末のデータは消えます。もう一度サインインすると、用意した記録帳と送り済みの記録は戻ります。"))
    }

    static func unsentAfterFlush(_ n: Int) -> String {
        L10n.tr("\(n) 件の記録を送れませんでした。送らずにログアウトすると、この記録は消えます。")
    }
}
