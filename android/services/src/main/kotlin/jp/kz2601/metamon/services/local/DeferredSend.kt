package jp.kz2601.metamon.services.local

import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Job
import kotlinx.coroutines.flow.first
import kotlinx.coroutines.launch

/**
 * 圏外で送れなかった **1 件** を持ち、オンライン復帰で **1 回だけ** 流す(iOS の `DeferredSend`・#60)。
 * 新しい依頼が来たら前の保留は置き換える(溜めない)。[cancel] で取り消せる。生成の依頼に使う。
 */
class DeferredSend<P>(
    private val connectivity: ConnectivityMonitor,
    private val scope: CoroutineScope,
) {
    private val lock = Any()
    private var pendingPayload: P? = null
    private var waiter: Job? = null
    private var generation = 0

    val isOnline: Boolean get() = connectivity.isOnline

    /** 保留中の 1 件。 */
    val pending: P? get() = synchronized(lock) { pendingPayload }

    /**
     * オンラインなら false を返して何もしない(呼ぶ側がすぐ送る)。
     * オフラインなら保留して true を返し、復帰した時に [send] を 1 回呼ぶ(差し替え・取り消しされていれば呼ばない)。
     */
    fun holdIfOffline(payload: P, send: suspend (P) -> Unit): Boolean {
        if (connectivity.isOnline) return false
        val gen = synchronized(lock) {
            generation += 1
            pendingPayload = payload
            waiter?.cancel()
            waiter = null
            generation
        }
        val job = scope.launch {
            connectivity.online.first { it }
            val fire = synchronized(lock) {
                if (generation != gen) return@synchronized null
                pendingPayload.also { pendingPayload = null; waiter = null }
            }
            if (fire != null) send(fire)
        }
        synchronized(lock) { if (generation == gen && pendingPayload != null) waiter = job }
        return true
    }

    /** 保留を取り消す(復帰しても送らない)。 */
    fun cancel() = synchronized(lock) {
        generation += 1
        pendingPayload = null
        waiter?.cancel()
        waiter = null
    }
}
