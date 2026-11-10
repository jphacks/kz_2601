package jp.kz2601.metamon.services.live

import android.content.Context
import android.util.Log
import com.google.android.gms.tasks.Task
import com.google.firebase.FirebaseApp
import com.google.firebase.auth.FirebaseAuth
import com.google.firebase.auth.FirebaseUser
import jp.kz2601.metamon.services.AuthService
import jp.kz2601.metamon.services.AuthUser
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.suspendCancellableCoroutine
import kotlin.coroutines.resume
import kotlin.coroutines.resumeWithException

/** Firebase の初期化。google-services.json が無いビルドでも落とさず false を返す(iOS の `FirebaseBootstrap`)。 */
object FirebaseBootstrap {
    @Volatile
    var isConfigured: Boolean = false
        private set

    fun configureIfAvailable(context: Context): Boolean {
        if (isConfigured) return true
        // google-services プラグインが json から作るリソースが無ければ null が返る
        isConfigured = FirebaseApp.initializeApp(context) != null
        if (!isConfigured) {
            Log.w(
                "Metamon",
                "google-services.json がありません。android/scripts/sync-google-services.sh を実行してビルドし直してください。認証はモック(固定 uid)で動きます",
            )
        }
        return isConfigured
    }
}

/** Firebase Auth(匿名。Google・Apple は #356 の続きで足す)。 */
class FirebaseAuthService private constructor(private val auth: FirebaseAuth) : AuthService {
    override val kind: String = "firebase"

    private val user = MutableStateFlow(auth.currentUser?.toAuthUser())
    override val currentUser: StateFlow<AuthUser?> = user

    init {
        auth.addAuthStateListener { user.value = it.currentUser?.toAuthUser() }
    }

    override suspend fun signInAnonymously(): AuthUser {
        val result = auth.signInAnonymously().await()
        return requireNotNull(result.user).toAuthUser()
    }

    override suspend fun idToken(forceRefresh: Boolean): String? =
        auth.currentUser?.getIdToken(forceRefresh)?.await()?.token

    override suspend fun signOut() = auth.signOut()

    companion object {
        /** Firebase が未初期化なら null(呼ぶ側がモックに落とす)。 */
        fun createIfConfigured(): FirebaseAuthService? =
            if (FirebaseBootstrap.isConfigured) FirebaseAuthService(FirebaseAuth.getInstance()) else null
    }
}

private fun FirebaseUser.toAuthUser() = AuthUser(uid = uid, isAnonymous = isAnonymous)

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
