package jp.kz2601.metamon.services.social

import jp.kz2601.metamon.dsl.records.InterestState
import jp.kz2601.metamon.dsl.records.MatchBox
import jp.kz2601.metamon.dsl.records.MatchInterest
import jp.kz2601.metamon.services.consent.KeyValueStore
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import kotlinx.coroutines.flow.update

/** 1 つのアプリの「届いた / 送った / つながった」の今。 */
data class MatchingSnapshot(
    /** box → 一覧。キーが無い = まだ読んでいない */
    val boxes: Map<MatchBox, List<MatchInterest>> = emptyMap(),
    /** 読めなかった box(行の代わりに「読み込めませんでした」) */
    val failed: Set<MatchBox> = emptySet(),
    /** 送信中の相手(ボタンを押せなくする) */
    val busy: Set<String> = emptySet(),
) {
    /** 相手 uid とのつながり。読んでいない box は無いものとして扱う。 */
    fun state(uid: String): InterestState = when {
        boxes[MatchBox.MATCHES].orEmpty().any { it.uid == uid } -> InterestState.MATCHED
        boxes[MatchBox.OUTGOING].orEmpty().any { it.uid == uid } -> InterestState.SENT
        boxes[MatchBox.INCOMING].orEmpty().any { it.uid == uid } -> InterestState.INCOMING
        else -> InterestState.NONE
    }
}

/**
 * 1 つのアプリの仲間探しの一覧(iOS の `MatchingStore`・#507)。操作は楽観的に先に反映し、失敗したら操作前に戻して throw する。
 * サーバーが正: 成功した応答と [load] の結果で置き換える。
 */
class MatchingStore(val appId: String, private val client: MatchingClient) {
    private val _snapshot = MutableStateFlow(MatchingSnapshot())
    val snapshot: StateFlow<MatchingSnapshot> = _snapshot.asStateFlow()

    fun items(box: MatchBox): List<MatchInterest>? = _snapshot.value.boxes[box]

    fun state(uid: String): InterestState = _snapshot.value.state(uid)

    fun isBusy(uid: String): Boolean = uid in _snapshot.value.busy

    suspend fun load(box: MatchBox) {
        try {
            val items = client.list(appId, box)
            _snapshot.update { it.copy(boxes = it.boxes + (box to items), failed = it.failed - box) }
        } catch (e: Exception) {
            _snapshot.update { if (box in it.boxes) it else it.copy(failed = it.failed + box) }
            throw e
        }
    }

    /** 相手のプロフィール([recordId]・作者 [uid])に送る。相手から届いていれば、その場でつながる。 */
    suspend fun send(uid: String, recordId: String, fromRecordId: String): MatchInterest = optimistic(uid, apply = { b ->
        val hit = b[MatchBox.INCOMING].orEmpty().firstOrNull { it.uid == uid }
        if (hit != null) insert(remove(b, uid, MatchBox.INCOMING), hit.copy(status = MatchInterest.MATCHED), MatchBox.MATCHES)
        else insert(b, MatchInterest(uid, profileRecordId = recordId), MatchBox.OUTGOING)
    }) { client.sendInterest(appId, recordId, fromRecordId, ageConfirmed = true) }

    /** 届いたものを承認する → つながる。 */
    suspend fun accept(uid: String): MatchInterest = optimistic(uid, apply = { b ->
        val hit = b[MatchBox.INCOMING].orEmpty().firstOrNull { it.uid == uid } ?: return@optimistic b
        insert(remove(b, uid, MatchBox.INCOMING), hit.copy(status = MatchInterest.MATCHED), MatchBox.MATCHES)
    }) { client.accept(appId, uid, ageConfirmed = true) }

    /** 届いたものを見送る(相手には知らされない)。 */
    suspend fun decline(uid: String) = mutate(uid, { remove(it, uid, MatchBox.INCOMING) }) { client.decline(appId, uid) }

    /** 送ったものを取り下げる。 */
    suspend fun withdraw(uid: String) = mutate(uid, { remove(it, uid, MatchBox.OUTGOING) }) { client.decline(appId, uid) }

    /** つながりを解除する。 */
    suspend fun unmatch(uid: String) = mutate(uid, { remove(it, uid, MatchBox.MATCHES) }) { client.unmatch(appId, uid) }

    /** ブロックした人を全部の一覧から消す(サーバーも返さなくなる)。 */
    fun forget(uid: String) = _snapshot.update { s -> s.copy(boxes = MatchBox.entries.fold(s.boxes) { b, box -> remove(b, uid, box) }) }

    private suspend fun mutate(uid: String, apply: (Boxes) -> Boxes, call: suspend () -> Unit) {
        val before = _snapshot.value.boxes
        _snapshot.update { it.copy(boxes = apply(it.boxes), busy = it.busy + uid) }
        try {
            call()
        } catch (e: Throwable) {
            _snapshot.update { it.copy(boxes = before) }
            throw e
        } finally {
            _snapshot.update { it.copy(busy = it.busy - uid) }
        }
    }

    private suspend fun optimistic(uid: String, apply: (Boxes) -> Boxes, call: suspend () -> MatchInterest): MatchInterest {
        var out: MatchInterest? = null
        mutate(uid, apply) {
            val result = call()
            out = result
            _snapshot.update { it.copy(boxes = settle(it.boxes, result)) }
        }
        return out ?: MatchInterest(uid)
    }

    companion object {
        private fun settle(boxes: Boxes, result: MatchInterest): Boxes {
            val cleared = MatchBox.entries.fold(boxes) { b, box -> remove(b, result.uid, box) }
            return insert(cleared, result, if (result.isMatched) MatchBox.MATCHES else MatchBox.OUTGOING)
        }

        /** 新しいものを先頭に。読んでいない box にも足す(行のボタンがすぐ変わる。次の [load] でサーバーの値に置き換わる)。 */
        private fun insert(boxes: Boxes, item: MatchInterest, box: MatchBox): Boxes =
            boxes + (box to listOf(item) + boxes[box].orEmpty().filter { it.uid != item.uid })

        private fun remove(boxes: Boxes, uid: String, box: MatchBox): Boxes {
            val list = boxes[box] ?: return boxes
            return if (list.none { it.uid == uid }) boxes else boxes + (box to list.filter { it.uid != uid })
        }
    }
}

private typealias Boxes = Map<MatchBox, List<MatchInterest>>

/** 「18 歳以上です」の確認(#507)。初回だけ聞き、端末に覚える。年齢・生年月日は保存しない。鍵は iOS と同じ。 */
class MatchingAgeGate(private val store: KeyValueStore) {
    val isConfirmed: Boolean get() = store.getBoolean(KEY) == true

    fun confirm() = store.putBoolean(KEY, true)

    /** サーバーが確認を求めた(400 age_unconfirmed)時に、次は聞き直す。 */
    fun reset() = store.remove(KEY)

    companion object {
        const val KEY = "matching.ageConfirmed.v1"
    }
}
