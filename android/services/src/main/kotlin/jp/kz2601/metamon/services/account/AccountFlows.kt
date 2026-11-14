package jp.kz2601.metamon.services.account

import jp.kz2601.metamon.services.AuthError
import jp.kz2601.metamon.services.AuthService
import kotlinx.coroutines.CancellationException

/**
 * アカウント削除の順番(#841・iOS の `AccountDeletionFlow`)。画面(再認証のシート・端末の後片付け)は呼ぶ側が渡す。
 *
 * 1. `DELETE /v1/me`(サーバーで論理削除)。失敗したら throw して何もしない(やり直せる)
 * 2. Firebase のユーザーを消す。入り直しが要れば [reauthenticate] してもう一度。それでも駄目ならログだけ(サーバーは削除済み)
 * 3. 端末のデータを消す
 * 4. サインアウト(2 が成功していれば Firebase 側は既にサインアウト済み)
 */
object AccountDeletionFlow {
    /** @param reauthenticate 入り直せたら true(匿名・キャンセルは false) */
    suspend fun run(
        account: AccountClient,
        auth: AuthService,
        reauthenticate: suspend () -> Boolean,
        wipeLocal: suspend () -> Unit,
        log: (String) -> Unit = { println("[Metamon] $it") },
    ) {
        account.deleteAccount()
        try {
            try {
                auth.deleteCurrentUser()
            } catch (e: AuthError.RequiresRecentLogin) {
                if (!reauthenticate()) throw e
                auth.deleteCurrentUser()
            }
        } catch (e: CancellationException) {
            throw e
        } catch (e: Exception) {
            log("Firebase user delete failed (server already deleted; signing out): $e")
        }
        wipeLocal()
        try {
            auth.signOut()
        } catch (e: CancellationException) {
            throw e
        } catch (e: Exception) {
            log("signOut after delete failed: $e")
        }
    }
}

/**
 * ログアウト(iOS の `SignOutFlow`)。アカウント削除と同じ後片付けで端末のデータを消してからサインアウトする
 * (次にこの端末で入る人に前の人のアプリや記録が見えない・前の人の未送信が次の人のアカウントで送られない)。
 */
object SignOutFlow {
    /** 確認に出す内容。 */
    sealed interface Prompt {
        /** 未送信なし */
        data object Plain : Prompt

        /** 未送信あり。[canSend] = 今送れる(オンライン)。送れる時は「送ってからログアウト」を出す */
        data class Unsent(val count: Int, val canSend: Boolean) : Prompt
    }

    enum class Choice {
        /** 未送信を送ってから消す(未送信が無ければ普通に消す) */
        SEND_THEN_WIPE,

        /** 未送信を捨てて消す */
        DISCARD_AND_WIPE,
    }

    sealed interface Outcome {
        data object SignedOut : Outcome

        /** 送り切れなかった件数。何も消さず、サインインしたまま */
        data class Unsent(val count: Int) : Outcome
    }

    fun prompt(pending: Int, isOnline: Boolean): Prompt = if (pending > 0) Prompt.Unsent(pending, isOnline) else Prompt.Plain

    /** @param flush 未送信を送り、残った件数を返す */
    suspend fun run(
        choice: Choice,
        auth: AuthService,
        flush: suspend () -> Int,
        wipeLocal: suspend () -> Unit,
    ): Outcome {
        if (choice == Choice.SEND_THEN_WIPE) {
            val left = flush()
            if (left > 0) return Outcome.Unsent(left)
        }
        wipeLocal()
        auth.signOut()
        return Outcome.SignedOut
    }
}
