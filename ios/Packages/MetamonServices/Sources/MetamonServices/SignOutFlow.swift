import Foundation

/// サインアウト。アカウント削除と同じ後片付けで端末のデータを消してからサインアウトする
/// (次にこの端末でサインインする人に、前の人のアプリや記録が見えない・前の人の未送信が次の人のアカウントで送られない)。
/// 端末の後片付けは呼び出し側がクロージャで渡す(`AppRuntime.wipeLocalData`)。
public enum SignOutFlow {
    /// 確認に出す内容。
    public enum Prompt: Equatable, Sendable {
        /// 未送信なし
        case plain
        /// 未送信あり。`canSend` = 今送れる(オンライン)。送れる時は「送ってから消す」を既定にする
        case unsent(count: Int, canSend: Bool)
    }

    public enum Choice: Equatable, Sendable {
        /// 未送信を送ってから消す(未送信が無ければ普通に消す)
        case sendThenWipe
        /// 未送信を捨てて消す
        case discardAndWipe
    }

    public enum Outcome: Equatable, Sendable {
        case signedOut
        /// 送り切れなかった件数。何も消さず、サインインしたまま
        case unsent(Int)
    }

    public static func prompt(pending: Int, isOnline: Bool) -> Prompt {
        pending > 0 ? .unsent(count: pending, canSend: isOnline) : .plain
    }

    /// - Parameter flush: 未送信を送り、残った件数を返す
    @MainActor
    public static func run(
        choice: Choice,
        auth: any AuthService,
        flush: @MainActor () async -> Int,
        wipeLocal: @MainActor () async -> Void
    ) async throws -> Outcome {
        if choice == .sendThenWipe {
            let left = await flush()
            if left > 0 { return .unsent(left) }
        }
        await wipeLocal()
        try auth.signOut()
        return .signedOut
    }
}
