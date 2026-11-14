package jp.kz2601.metamon.account

import jp.kz2601.metamon.services.AuthUser
import kotlinx.coroutines.CancellationException

/**
 * アカウント削除の前に Apple のトークンを取り消す(#1165・iOS の `AppleTokenRevocation`)。
 * App Store / Apple の決まりで、Apple でサインインした人を消したら Apple 側の「Apple でサインイン」の一覧からも外す。
 *
 * iOS は Apple の再認証シートから `authorizationCode` を受け取って Firebase の `revokeToken` に渡す。
 * Android の Apple は Firebase の web フロー(Custom Tab)なので、authorization code は Firebase(Google のサーバー)が
 * 自分で交換してしまい端末には来ない。端末に来るのは交換後の **Apple の access token**
 * ([com.google.firebase.auth.OAuthCredential.getAccessToken])で、これを `FirebaseAuth.revokeAccessToken` に渡すと
 * Firebase が Apple の `auth/revoke` を呼ぶ(docs/legal/apple-token-revocation.md と同じ仕組み)。
 *
 * 取り消しは **Firebase のユーザーを消す前**に行う(消した後は本人として取り消せない)。
 * 再認証・取り消しのどちらが失敗しても例外にせず、削除は続ける([Outcome] を見ればどこまで進んだか分かる)。
 */
object AppleTokenRevocation {
    /** どこまで進んだか。両方 false でも削除は続ける。 */
    data class Outcome(val reauthenticated: Boolean = false, val revoked: Boolean = false) {
        companion object {
            /** Apple で入った人ではないので何もしない。 */
            val NotApple = Outcome()
        }
    }

    internal const val REAUTH_FAILED = "Apple reauthenticate failed (continuing)"
    internal const val REVOKE_FAILED = "Apple token revoke failed (continuing)"
    internal const val NO_TOKEN = "Apple access token is missing (continuing)"

    /**
     * @param provider 今の人が入った方法([AuthUser.provider])。[AuthUser.APPLE] 以外は何もしない
     * @param reauthenticate Apple の再認証をして access token を返す(取れなければ null)。Firebase の入り直しも兼ねる
     * @param revoke [reauthenticate] で取れたトークンを取り消す(`FirebaseAuth.revokeAccessToken`)
     */
    suspend fun run(
        provider: String?,
        reauthenticate: suspend () -> String?,
        revoke: suspend (String) -> Unit,
        log: (String) -> Unit = { println("[Metamon] $it") },
    ): Outcome {
        if (provider != AuthUser.APPLE) return Outcome.NotApple
        val token = try {
            reauthenticate()
        } catch (e: CancellationException) {
            throw e
        } catch (e: Exception) {
            log("$REAUTH_FAILED: $e")
            return Outcome()
        }
        if (token.isNullOrBlank()) {
            log(NO_TOKEN)
            return Outcome(reauthenticated = token != null)
        }
        return try {
            revoke(token)
            Outcome(reauthenticated = true, revoked = true)
        } catch (e: CancellationException) {
            throw e
        } catch (e: Exception) {
            // Firebase の Apple プロバイダに OAuth コードフロー(Services ID・Team ID・Key ID・秘密鍵)が無いと失敗する
            log("$REVOKE_FAILED: $e")
            Outcome(reauthenticated = true)
        }
    }
}
