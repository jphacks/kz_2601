package jp.kz2601.metamon.account

import jp.kz2601.metamon.services.AuthService
import jp.kz2601.metamon.services.AuthUser
import jp.kz2601.metamon.services.http.TokenProvider
import jp.kz2601.metamon.services.local.AccountOwnerStore
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock

/** 端末のデータを今使ってよい uid(後片付けを済ませて受け入れた人)。backend へのトークンはこの人の分だけ付ける。 */
class ActiveOwner {
    @Volatile
    var uid: String? = null
        internal set
}

/**
 * backend への Bearer を [ActiveOwner] の人の分だけ付ける(#841)。
 * uid が変わってから後片付けが済むまでの間は付けない = 前の人の送信待ちが次の人のトークンで送られない。
 */
class OwnerGatedTokens(private val auth: AuthService, private val owner: ActiveOwner) : TokenProvider {
    override suspend fun idToken(forceRefresh: Boolean): String? {
        val uid = owner.uid ?: return null
        if (auth.currentUser.value?.uid != uid) return null
        return auth.idToken(forceRefresh)
    }
}

/**
 * 端末のデータの持ち主を見張る(#841)。サインアウト(外からも)・uid が変わった時に必ず [wipe] を走らせ、
 * 記録の同期はサインインして持ち主を受け入れた後だけ動かす。
 *
 * - サインアウト: 同期を止め、前の人のデータがあれば消す
 * - 別の uid: 同期を止め、トークンを止め、前の人のデータを消してから受け入れる(起動をまたいでも [saved] で分かる)
 * - 同じ uid(匿名 → 引き継ぎなど): そのまま
 *
 * 画面は [active] で本体を出す(後片付けが済む前に前の人の画面を出さない)。
 */
class AccountGuard(
    private val auth: AuthService,
    private val saved: AccountOwnerStore,
    val owner: ActiveOwner,
    private val startSync: () -> Unit,
    private val stopSync: () -> Unit,
    private val wipe: suspend () -> Unit,
) {
    private val _active = MutableStateFlow<AuthUser?>(null)

    /** 受け入れたサインイン中の人。null の間はサインイン画面(後片付け中を含む)。 */
    val active: StateFlow<AuthUser?> = _active.asStateFlow()

    private val _generation = MutableStateFlow(0)

    /** 後片付けの回数。画面の状態(音声の ViewModel など)を作り直すキーに使う。 */
    val generation: StateFlow<Int> = _generation.asStateFlow()

    private val lock = Mutex()
    private var job: Job? = null

    /** サインイン状態の購読を始める。2 回呼んでも 1 回だけ。 */
    fun start(scope: CoroutineScope) {
        if (job != null) return
        job = scope.launch { auth.currentUser.collect { reconcile(it) } }
    }

    internal suspend fun reconcile(user: AuthUser?) = lock.withLock {
        val uid = user?.uid
        if (uid == null) {
            owner.uid = null
            stopSync()
            _active.value = null
            if (saved.uid != null) wipeLocked()
            return@withLock
        }
        if (owner.uid == uid) {
            _active.value = user // 匿名 → 引き継ぎ(uid は同じ)
            return@withLock
        }
        owner.uid = null
        _active.value = null
        stopSync()
        val previous = saved.uid
        if (previous != null && previous != uid) wipeLocked()
        saved.uid = uid
        owner.uid = uid
        startSync()
        _active.value = user
    }

    /**
     * ログアウト・アカウント削除の後片付け(サインアウトの前に呼ぶ)。トークンと同期を止めて消し、持ち主を忘れる。
     * 続くサインアウトで [reconcile] がもう一度消すことはない。
     */
    suspend fun wipeForSignOut() = lock.withLock {
        owner.uid = null
        stopSync()
        wipeLocked()
    }

    private suspend fun wipeLocked() {
        wipe()
        saved.uid = null
        _generation.value += 1
    }
}
