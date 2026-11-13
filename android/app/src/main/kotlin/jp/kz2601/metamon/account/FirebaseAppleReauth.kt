package jp.kz2601.metamon.account

import android.app.Activity
import com.google.android.gms.tasks.Task
import com.google.firebase.auth.FirebaseAuth
import com.google.firebase.auth.OAuthCredential
import com.google.firebase.auth.OAuthProvider
import jp.kz2601.metamon.services.AuthUser
import jp.kz2601.metamon.services.live.FirebaseBootstrap
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlin.coroutines.resume
import kotlin.coroutines.resumeWithException

/**
 * 削除の前の Apple の入り直しと、Apple のトークンの取り消し(#1165)。[AppleTokenRevocation] に渡す。
 *
 * `AuthService` は入り直しの結果を返さないので、access token が要るここだけ Firebase を直に使う。
 * Firebase が未初期化(google-services.json 無し・認証はモック)のビルドでは [createIfConfigured] が null を返し、
 * 取り消しは行わない(削除はそのまま進む)。
 */
class FirebaseAppleReauth internal constructor(private val auth: FirebaseAuth) {
    /**
     * Apple のページ(Custom Tab)を出して入り直し、Apple の access token を返す。
     * 取れなければ null(未サインイン・資格情報が OAuth でない)。
     */
    suspend fun reauthenticate(activity: Activity): String? {
        val user = auth.currentUser ?: return null
        val provider = OAuthProvider.newBuilder(AuthUser.APPLE).build()
        val result = user.startActivityForReauthenticateWithProvider(activity, provider).await()
        return (result.credential as? OAuthCredential)?.accessToken
    }

    /** Firebase に Apple の `auth/revoke` を呼んでもらう(client secret は Firebase が作る)。 */
    suspend fun revoke(accessToken: String) {
        auth.revokeAccessToken(accessToken).await()
    }

    companion object {
        /** Firebase が未初期化なら null。 */
        fun createIfConfigured(): FirebaseAppleReauth? =
            if (FirebaseBootstrap.isConfigured) FirebaseAppleReauth(FirebaseAuth.getInstance()) else null
    }
}

/** Play services の Task を待つ(`services` の内部版と同じ最小実装)。 */
private suspend fun <T> Task<T>.await(): T = suspendCancellableCoroutine { cont ->
    addOnCompleteListener { task ->
        val error = task.exception
        when {
            error != null -> cont.resumeWithException(error)
            task.isCanceled -> cont.cancel()
            else -> cont.resume(task.result)
        }
    }
}
