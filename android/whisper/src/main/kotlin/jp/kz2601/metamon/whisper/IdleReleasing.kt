package jp.kz2601.metamon.whisper

import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.NonCancellable
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withContext

/**
 * 重いもの(whisper のモデル)を使う時だけ読み込み、最後に使ってから [idleMillis] 経ったら手放す。
 * 使っている途中には手放さない。[releaseWhenIdle] はすぐ(使っていればその後すぐ)手放す。
 */
class IdleReleasing<T : Any>(
    private val scope: CoroutineScope,
    private val idleMillis: Long,
    private val load: suspend () -> T,
    private val release: suspend (T) -> Unit,
) {
    private val mutex = Mutex()
    private var value: T? = null
    private var users = 0
    private var releasePending = false
    private var idleJob: Job? = null

    /** 読み込み済みか(テストと表示用)。 */
    val isLoaded: Boolean get() = value != null

    /** 読み込んで(済みならそのまま)[block] に渡す。終わったら手放す時計を回し直す。 */
    suspend fun <R> use(block: suspend (T) -> R): R {
        val v = mutex.withLock {
            idleJob?.cancel()
            idleJob = null
            val v = value ?: load().also { value = it }
            users++
            v
        }
        try {
            return block(v)
        } finally {
            withContext(NonCancellable) {
                mutex.withLock {
                    users--
                    if (users == 0) {
                        if (releasePending) releaseLocked() else scheduleIdleLocked()
                    }
                }
            }
        }
    }

    /** 使っていなければ今すぐ、使っていれば終わり次第手放す(メモリ不足・サインアウト)。 */
    suspend fun releaseWhenIdle() = mutex.withLock {
        if (users == 0) releaseLocked() else releasePending = true
    }

    private fun scheduleIdleLocked() {
        idleJob?.cancel()
        idleJob = scope.launch {
            delay(idleMillis)
            mutex.withLock {
                if (users == 0 && idleJob === coroutineContext[Job]) releaseLocked()
            }
        }
    }

    private suspend fun releaseLocked() {
        idleJob?.cancel()
        idleJob = null
        releasePending = false
        val v = value ?: return
        value = null
        // 手放す途中で止められて(待ちの job が自分を cancel した時も)モデルが漏れないように
        withContext(NonCancellable) { release(v) }
    }
}
