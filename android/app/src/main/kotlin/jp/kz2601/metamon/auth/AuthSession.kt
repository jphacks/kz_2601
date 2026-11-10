package jp.kz2601.metamon.auth

import android.app.Activity
import jp.kz2601.metamon.services.AuthError
import jp.kz2601.metamon.services.AuthService
import jp.kz2601.metamon.services.AuthUser
import jp.kz2601.metamon.services.account.AccountClient
import jp.kz2601.metamon.services.account.AccountDeletionFlow
import jp.kz2601.metamon.services.account.SignOutFlow
import jp.kz2601.metamon.services.local.RecordSyncEngine
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

/**
 * サインイン状態と操作(iOS の `AuthSession`)。画面は [user] で [RootScreen] を決める。
 * [beforeSignOut] はサインアウトの前に呼ぶ(通知の端末の登録を消す・#887)。失敗してもサインアウトは続ける。
 */
class AuthSession(val service: AuthService, private val beforeSignOut: suspend () -> Unit = {}) {
    val user: StateFlow<AuthUser?> = service.currentUser

    private val busy = MutableStateFlow(false)
    val isBusy: StateFlow<Boolean> = busy

    private val error = MutableStateFlow<String?>(null)
    val errorMessage: StateFlow<String?> = error

    suspend fun signInAnonymously() = perform { service.signInAnonymously() }

    suspend fun signInWithGoogle(source: GoogleIdTokenSource) = perform { service.signInWithGoogle(source.idToken()) }

    suspend fun signInWithApple(activity: Activity) = perform { service.signInWithApple(activity) }

    suspend fun signOut() {
        runBeforeSignOut()
        runCatching { service.signOut() }.onFailure { error.value = it.message }
    }

    /** 失敗してもサインアウトは続ける(次のサインインで登録し直す)。トークンを止める前に呼ぶ。 */
    private suspend fun runBeforeSignOut() {
        try {
            beforeSignOut()
        } catch (e: CancellationException) {
            throw e
        } catch (_: Exception) {
        }
    }

    /**
     * ログアウト(#841・iOS の `signOutAndWipe`)。取り消し待ちの削除は今のアカウントで送る扱いにしてから、
     * [SignOutFlow] で(送ってから)通知の登録 → 端末のデータの順に消してサインアウトする。
     * @return [SignOutFlow.Outcome.Unsent] なら送り切れず、何も消していない(サインインしたまま)
     */
    suspend fun signOutAndWipe(
        choice: SignOutFlow.Choice,
        recordSync: RecordSyncEngine,
        wipe: suspend () -> Unit,
    ): SignOutFlow.Outcome = SignOutFlow.run(choice, service, flush = { recordSync.flush() }, wipeLocal = {
        // 通知の登録を先に消す(後片付けがトークンを止めるので、その前)・#887
        runBeforeSignOut()
        wipe()
    })

    /** ログアウトの確認に出す内容(取り消し待ちの削除も未送信に数える)。 */
    suspend fun signOutPrompt(recordSync: RecordSyncEngine): SignOutFlow.Prompt {
        recordSync.commitPendingDeletions()
        return SignOutFlow.prompt(recordSync.pendingCount.value, recordSync.isOnline)
    }

    /**
     * アカウント削除(#841・iOS の `deleteAccount`)。`DELETE /v1/me` → Firebase のユーザー削除(要れば入り直し)
     * → 端末のデータ → サインアウト。失敗(サーバーに届かない等)は throw して何も消さない。
     * @param reauthenticate 入り直せたら true。匿名・キャンセルは false
     */
    suspend fun deleteAccount(account: AccountClient, reauthenticate: suspend () -> Boolean, wipe: suspend () -> Unit) {
        // 通知の登録はアカウントを消す前に消す(消した後はトークンが通らない)・#887
        runBeforeSignOut()
        AccountDeletionFlow.run(account, service, reauthenticate = reauthenticate, wipeLocal = wipe)
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
