package jp.kz2601.metamon.auth

import android.app.Activity
import jp.kz2601.metamon.services.AuthError
import jp.kz2601.metamon.services.AuthService
import jp.kz2601.metamon.services.AuthUser
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow

/** 起動後にどちらの画面を出すか(iOS の `user == nil ? SignInView : RootView`)。 */
enum class RootScreen {
    SignIn,
    App,
    ;

    companion object {
        fun of(user: AuthUser?): RootScreen = if (user == null) SignIn else App
    }
}

/** Credential Manager から Google の ID トークンを取る口(画面側が Activity を持って作る)。 */
fun interface GoogleIdTokenSource {
    suspend fun idToken(): String
}

/** サインイン状態と操作(iOS の `AuthSession`)。画面は [user] で [RootScreen] を決める。 */
class AuthSession(val service: AuthService) {
    val user: StateFlow<AuthUser?> = service.currentUser

    private val busy = MutableStateFlow(false)
    val isBusy: StateFlow<Boolean> = busy

    private val error = MutableStateFlow<String?>(null)
    val errorMessage: StateFlow<String?> = error

    suspend fun signInAnonymously() = perform { service.signInAnonymously() }

    suspend fun signInWithGoogle(source: GoogleIdTokenSource) = perform { service.signInWithGoogle(source.idToken()) }

    suspend fun signInWithApple(activity: Activity) = perform { service.signInWithApple(activity) }

    suspend fun signOut() {
        runCatching { service.signOut() }.onFailure { error.value = it.message }
    }

    private suspend fun perform(body: suspend () -> AuthUser) {
        if (busy.value) return
        busy.value = true
        error.value = null
        try {
            body()
        } catch (e: CancellationException) {
            throw e
        } catch (_: AuthError.Canceled) {
            // 自分で閉じた: 何も出さない
        } catch (e: Exception) {
            error.value = e.message ?: SIGN_IN_FAILED
        } finally {
            busy.value = false
        }
    }

    companion object {
        const val SIGN_IN_FAILED = "サインインできませんでした。もう一度お試しください"
    }
}
