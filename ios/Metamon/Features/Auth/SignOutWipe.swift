import Foundation
import MetamonServices

extension AuthSession {
    /// 設定の「ログアウト」。アカウント削除と同じ後片付け(`wipeLocalData`)で端末のデータを消してからサインアウトする。
    /// - Returns: `.unsent(n)` なら送り切れず、何も消していない(サインインしたまま)
    @discardableResult
    func signOutAndWipe(runtime: AppRuntime, choice: SignOutFlow.Choice) async throws -> SignOutFlow.Outcome {
        let wasSignedIn = user != nil
        await runtime.settlePendingRecordDeletes() // 取り消し待ちの削除はいまのアカウントで送る(送らないなら下の wipe で消える)
        let outcome = try await SignOutFlow.run(
            choice: choice,
            auth: service,
            flush: { await runtime.recordSync.flush() },
            wipeLocal: {
                await runtime.unregisterPushDevice() // 送れなくても続ける(#269)
                await runtime.wipeLocalData()
            }
        )
        guard outcome == .signedOut else { return outcome }
        signOut() // セッションの表示を即反映(サインアウト済みでも安全)
        farewell = wasSignedIn // #102
        return outcome
    }
}
