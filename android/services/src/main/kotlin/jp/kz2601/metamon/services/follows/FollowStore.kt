package jp.kz2601.metamon.services.follows

import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update
import java.util.concurrent.atomic.AtomicBoolean

/** 1 つのアプリのフォローの今。 */
data class FollowsSnapshot(
    /** フォローしている人。null = まだ読めていない */
    val uids: Set<String>? = null,
    /** 送信中の相手(二度押しを捨てる) */
    val busy: Set<String> = emptySet(),
)

/**
 * 1 つのアプリのフォロー一覧(iOS の `FollowStore`・#270)。操作は楽観的に先に反映し、失敗したら戻して throw する。
 * サーバーの応答が正。
 */
class FollowStore(val appId: String, private val client: FollowsClient) {
    private val _snapshot = MutableStateFlow(FollowsSnapshot())
    val snapshot: StateFlow<FollowsSnapshot> = _snapshot.asStateFlow()
    private val loading = AtomicBoolean(false)

    val uids: Set<String>? get() = _snapshot.value.uids

    fun isFollowing(uid: String): Boolean = uids?.contains(uid) ?: false

    /** 読み込む(読み込み中の 2 回目は何もしない)。失敗したら前の一覧のまま throw。 */
    suspend fun load() {
        if (!loading.compareAndSet(false, true)) return
        try {
            val list = client.list(appId).toSet()
            _snapshot.update { it.copy(uids = list) }
        } finally {
            loading.set(false)
        }
    }

    /** フォローする / 外す。送信中の同じ相手には何もしない。 */
    suspend fun set(uid: String, on: Boolean) {
        val prev = _snapshot.value
        if (uid in prev.busy) return
        val before = prev.uids
        _snapshot.update { it.copy(uids = apply(it.uids, uid, on), busy = it.busy + uid) }
        try {
            val following = client.setFollow(appId, uid, on)
            _snapshot.update { it.copy(uids = apply(it.uids, uid, following)) }
        } catch (e: Throwable) {
            _snapshot.update { it.copy(uids = before) }
            throw e
        } finally {
            _snapshot.update { it.copy(busy = it.busy - uid) }
        }
    }

    private companion object {
        fun apply(uids: Set<String>?, uid: String, on: Boolean): Set<String> {
            val next = uids.orEmpty()
            return if (on) next + uid else next - uid
        }
    }
}
