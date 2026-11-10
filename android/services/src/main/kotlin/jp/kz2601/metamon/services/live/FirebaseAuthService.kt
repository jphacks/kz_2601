package jp.kz2601.metamon.services.live

import android.app.Activity
import android.content.Context
import android.util.Log
import com.google.firebase.FirebaseApp
import com.google.firebase.appcheck.AppCheckProviderFactory
import com.google.firebase.appcheck.FirebaseAppCheck
import com.google.firebase.auth.AuthCredential
import com.google.firebase.auth.AuthResult
import com.google.firebase.auth.FirebaseAuth
import com.google.firebase.auth.FirebaseAuthException
import com.google.firebase.auth.FirebaseAuthRecentLoginRequiredException
import com.google.firebase.auth.FirebaseAuthUserCollisionException
import com.google.firebase.auth.FirebaseUser
import com.google.firebase.auth.GoogleAuthProvider
import com.google.firebase.auth.OAuthProvider
import jp.kz2601.metamon.services.AccountLinker
import jp.kz2601.metamon.services.AuthError
import jp.kz2601.metamon.services.AuthService
import jp.kz2601.metamon.services.AuthUser
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow

/** Firebase の初期化。google-services.json が無いビルドでも落とさず false を返す(iOS の `FirebaseBootstrap`)。 */
object FirebaseBootstrap {
    @Volatile
    var isConfigured: Boolean = false
        private set

    /**
     * @param appCheckFactory App Check のプロバイダ(:app が build type で選ぶ・#318)。null なら App Check を使わない。
     */
    fun configureIfAvailable(context: Context, appCheckFactory: AppCheckProviderFactory? = null): Boolean = configure(
        // google-services プラグインが json から作るリソースが無ければ null が返る
        initialize = { FirebaseApp.initializeApp(context) != null },
        installAppCheck = { appCheckFactory?.let { FirebaseAppCheck.getInstance().installAppCheckProviderFactory(it) } },
        log = { message, error -> Log.w("Metamon", message, error) },
    )

    /** 中身(テスト用に Firebase を差し替えられる形)。初期化・App Check のどちらで throw しても起動は止めない。 */
    internal fun configure(
        initialize: () -> Boolean,
        installAppCheck: () -> Unit,
        log: (String, Throwable?) -> Unit,
    ): Boolean {
        if (isConfigured) return true
        isConfigured = runCatching(initialize).getOrElse {
            log("Firebase の初期化に失敗しました。認証はモックで動きます", it)
            false
        }
        if (!isConfigured) {
            log(
                "google-services.json がありません。android/scripts/sync-google-services.sh を実行してビルドし直してください。認証はモック(固定 uid)で動きます",
                null,
            )
            return false
        }
        runCatching(installAppCheck).onFailure { log("App Check を入れられませんでした(ヘッダ無しで送ります)", it) }
        return true
    }

    internal fun resetForTest() {
        isConfigured = false
    }
}

/** Firebase Auth(匿名・Google・Apple)。 */
class FirebaseAuthService private constructor(private val auth: FirebaseAuth) : AuthService {
    override val kind: String = "firebase"

    private val user = MutableStateFlow(auth.currentUser?.toAuthUser())
    override val currentUser: StateFlow<AuthUser?> = user

    init {
        auth.addAuthStateListener { user.value = it.currentUser?.toAuthUser() }
    }

    override suspend fun signInAnonymously(): AuthUser = auth.signInAnonymously().await().toAuthUser()

    override suspend fun signInWithGoogle(googleIdToken: String): AuthUser =
        auth.signInWithCredential(GoogleAuthProvider.getCredential(googleIdToken, null)).await().toAuthUser()

    override suspend fun signInWithApple(activity: Activity): AuthUser {
        val provider = OAuthProvider.newBuilder(APPLE_PROVIDER_ID)
            .setScopes(listOf("email", "name"))
            .build()
        // Custom Tab の間にプロセスが落ちて戻ってきた時は、その結果を先に受け取る(Firebase の決まり)
        val task = auth.pendingAuthResult ?: auth.startActivityForSignInWithProvider(activity, provider)
        return try {
            task.await().toAuthUser()
        } catch (e: FirebaseAuthException) {
            if (e.errorCode == WEB_CONTEXT_CANCELED) throw AuthError.Canceled(e) else throw e
        }
    }

    override suspend fun idToken(forceRefresh: Boolean): String? =
        auth.currentUser?.getIdToken(forceRefresh)?.await()?.token

    override suspend fun signOut() = auth.signOut()

    override suspend fun deleteCurrentUser() {
        val current = auth.currentUser ?: return
        try {
            current.delete().await()
        } catch (e: FirebaseAuthRecentLoginRequiredException) {
            throw AuthError.RequiresRecentLogin(e)
        }
    }

    override suspend fun reauthenticateWithGoogle(googleIdToken: String) {
        val current = auth.currentUser ?: return
        current.reauthenticate(GoogleAuthProvider.getCredential(googleIdToken, null)).await()
    }

    override suspend fun reauthenticateWithApple(activity: Activity) {
        val current = auth.currentUser ?: return
        val provider = OAuthProvider.newBuilder(APPLE_PROVIDER_ID).build()
        try {
            current.startActivityForReauthenticateWithProvider(activity, provider).await()
        } catch (e: FirebaseAuthException) {
            if (e.errorCode == WEB_CONTEXT_CANCELED) throw AuthError.Canceled(e) else throw e
        }
    }

    /** 引き継ぎ(#1101)の分岐。credential-already-in-use の時の資格情報は Firebase の updated credential(Apple の nonce は 1 回限り)。 */
    private val linker = AccountLinker<AuthCredential>(
        currentUser = { auth.currentUser?.toAuthUser() },
        classify = { e ->
            AccountLinker.classifyFirebase(
                (e as? FirebaseAuthException)?.errorCode,
                (e as? FirebaseAuthUserCollisionException)?.updatedCredential,
            )
        },
        signInWith = { auth.signInWithCredential(it).await().toAuthUser() },
    )

    override suspend fun linkWithGoogle(googleIdToken: String): AuthUser {
        val credential = GoogleAuthProvider.getCredential(googleIdToken, null)
        return linker.link(
            link = { linked(requireNotNull(auth.currentUser).linkWithCredential(credential).await()) },
            signIn = { signInWithGoogle(googleIdToken) },
            fallback = credential, // Google の ID トークンは切り替えにもう一度使える
        )
    }

    override suspend fun linkWithApple(activity: Activity): AuthUser = linker.link(
        link = {
            val provider = OAuthProvider.newBuilder(APPLE_PROVIDER_ID).setScopes(listOf("email", "name")).build()
            val task = auth.pendingAuthResult ?: requireNotNull(auth.currentUser).startActivityForLinkWithProvider(activity, provider)
            linked(task.await())
        },
        signIn = { signInWithApple(activity) },
    )

    override suspend fun switchToPendingAccount(): AuthUser = linker.switchToPending()

    /** link の後は uid が同じでサインイン状態の通知が来ないので、匿名でなくなったことを自分で流す。 */
    private suspend fun linked(result: AuthResult): AuthUser {
        val linkedUser = requireNotNull(result.user)
        runCatching { linkedUser.reload().await() } // isAnonymous / providerData を最新に
        return (auth.currentUser ?: linkedUser).toAuthUser().also { user.value = it }
    }

    private fun AuthResult.toAuthUser(): AuthUser = requireNotNull(user).toAuthUser()

    companion object {
        const val APPLE_PROVIDER_ID = "apple.com"
        private const val WEB_CONTEXT_CANCELED = "ERROR_WEB_CONTEXT_CANCELED"

        /** Firebase が未初期化なら null(呼ぶ側がモックに落とす)。 */
        fun createIfConfigured(): FirebaseAuthService? =
            if (FirebaseBootstrap.isConfigured) FirebaseAuthService(FirebaseAuth.getInstance()) else null
    }
}

private fun FirebaseUser.toAuthUser(): AuthUser {
    val ids = providerData.map { it.providerId }
    val provider = listOf(AuthUser.APPLE, AuthUser.GOOGLE).firstOrNull { it in ids }
    return AuthUser(uid = uid, isAnonymous = isAnonymous, provider = provider)
}
