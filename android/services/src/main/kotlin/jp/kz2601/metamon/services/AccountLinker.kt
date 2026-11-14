package jp.kz2601.metamon.services

import kotlinx.coroutines.CancellationException

/** link の失敗の分類(Firebase の例外をここに写す)。 */
sealed interface LinkFailure<out C> {
    /** credential-already-in-use。[credential] は既存アカウントへ入り直す資格情報(Firebase の updated credential) */
    data class InUse<C>(val credential: C?) : LinkFailure<C>

    data object AlreadyLinked : LinkFailure<Nothing>

    data object Canceled : LinkFailure<Nothing>

    data object Other : LinkFailure<Nothing>
}

/**
 * 匿名 → Google / Apple の引き継ぎ(#1101・iOS の `FirebaseAuthService.linkWithApple`)の分岐。Firebase を差し替えられる形。
 * - 未サインイン: そのままサインイン
 * - 匿名でない: [AuthError.AlreadyLinked]
 * - link 成功: uid はそのまま
 * - credential-already-in-use: 資格情報を持っておき [AuthError.CredentialAlreadyInUse]。[switchToPending] でそのアカウントに入る
 *
 * @param signInWith 持っておいた資格情報でサインインする
 */
class AccountLinker<C : Any>(
    private val currentUser: () -> AuthUser?,
    private val classify: (Throwable) -> LinkFailure<C>,
    private val signInWith: suspend (C) -> AuthUser,
) {
    private val lock = Any()
    private var pending: C? = null

    /**
     * @param link 今の匿名の利用者に結びつける
     * @param signIn 未サインインの時の普通のサインイン
     * @param fallback Firebase が更新済みの資格情報を返さなかった時に切り替えに使う資格情報(Google の ID トークンなど)
     */
    suspend fun link(link: suspend () -> AuthUser, signIn: suspend () -> AuthUser, fallback: C? = null): AuthUser {
        synchronized(lock) { pending = null }
        val user = currentUser() ?: return signIn()
        if (!user.isAnonymous) throw AuthError.AlreadyLinked()
        return try {
            link()
        } catch (e: CancellationException) {
            throw e
        } catch (e: AuthError) {
            throw e
        } catch (e: Exception) {
            when (val failure = classify(e)) {
                is LinkFailure.InUse -> {
                    synchronized(lock) { pending = failure.credential ?: fallback }
                    throw AuthError.CredentialAlreadyInUse(e)
                }
                LinkFailure.AlreadyLinked -> throw AuthError.AlreadyLinked(e)
                LinkFailure.Canceled -> throw AuthError.Canceled(e)
                LinkFailure.Other -> throw e
            }
        }
    }

    /** 持っておいた資格情報の既存アカウントに入る(1 回だけ使える)。 */
    suspend fun switchToPending(): AuthUser {
        val credential = synchronized(lock) { pending.also { pending = null } } ?: throw AuthError.NoPendingAccount()
        return signInWith(credential)
    }

    companion object {
        const val CREDENTIAL_ALREADY_IN_USE = "ERROR_CREDENTIAL_ALREADY_IN_USE"
        const val PROVIDER_ALREADY_LINKED = "ERROR_PROVIDER_ALREADY_LINKED"
        const val WEB_CONTEXT_CANCELED = "ERROR_WEB_CONTEXT_CANCELED"

        /** Firebase の errorCode から分類する(`FirebaseAuthException.errorCode`・`FirebaseAuthUserCollisionException.updatedCredential`)。 */
        fun <C> classifyFirebase(errorCode: String?, updatedCredential: C?): LinkFailure<C> = when (errorCode) {
            CREDENTIAL_ALREADY_IN_USE -> LinkFailure.InUse(updatedCredential)
            PROVIDER_ALREADY_LINKED -> LinkFailure.AlreadyLinked
            WEB_CONTEXT_CANCELED -> LinkFailure.Canceled
            else -> LinkFailure.Other
        }
    }
}
