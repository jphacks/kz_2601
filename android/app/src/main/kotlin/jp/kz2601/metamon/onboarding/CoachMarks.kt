package jp.kz2601.metamon.onboarding

import jp.kz2601.metamon.services.consent.KeyValueStore
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.flow.asStateFlow
import java.time.Instant
import java.time.ZoneId

/**
 * 見た目から分からない操作を、使える場面に初めて来た時に 1 つずつ教える吹き出し(#900・iOS の `CoachMark`・#1441)。
 * 並び順 = 出す順番。iOS の `switcherReorder`(長押しで並べ替え)は Android の一覧に長押しの並べ替えが無い(「…」の移動)ので持たない。
 */
enum class CoachMark(val key: String) {
    /** マイクのボタンの長押しで変身一覧へ戻る */
    FAB_LONG_PRESS("fabLongPress"),

    /** マイクのボタンをドラッグで動かす・端に寄せて隠す */
    FAB_DRAG("fabDrag"),
    ;

    companion object {
        /** マイクのボタン(FAB)に出すもの */
        val FAB: Set<CoachMark> = setOf(FAB_LONG_PRESS, FAB_DRAG)

        fun of(key: String?): CoachMark? = entries.firstOrNull { it.key == key }
    }
}

/** 吹き出しで起きたこと(iOS の `CoachMarkEvent`)。 */
enum class CoachMarkEvent { SHOWN, DISMISSED, NEVER_AGAIN }

/** どれを出すかの決まり(保存から切り離した判定・iOS の `CoachMarkSchedule`)。 */
object CoachMarkSchedule {
    /** 前に出したもの(閉じる前にアプリを閉じた時も、次はそれを出し直す) */
    data class LastShown(val mark: CoachMark, val atMillis: Long)

    /**
     * いま出すもの。
     * - 「もう出さない」を選んだら出さない
     * - 閉じた・実際に操作したものは出さない
     * - 出したまま閉じていないものがこの場面で使えるなら、それを続けて出す(一度に 1 つ)
     * - 新しいものは 1 日 1 つまで。設定の「もう一度見る」の間は続けて出す
     */
    fun next(
        eligible: Set<CoachMark>,
        done: Set<CoachMark>,
        optedOut: Boolean,
        replaying: Boolean,
        lastShown: LastShown?,
        nowMillis: Long,
        zone: ZoneId = ZoneId.systemDefault(),
    ): CoachMark? {
        if (optedOut) return null
        val candidates = CoachMark.entries.filter { it in eligible && it !in done }
        if (lastShown != null && lastShown.mark in candidates) return lastShown.mark
        if (!replaying && lastShown != null && sameDay(lastShown.atMillis, nowMillis, zone)) return null
        return candidates.firstOrNull()
    }

    private fun sameDay(a: Long, b: Long, zone: ZoneId): Boolean =
        Instant.ofEpochMilli(a).atZone(zone).toLocalDate() == Instant.ofEpochMilli(b).atZone(zone).toLocalDate()
}

/**
 * 吹き出しを閉じたか・操作したか・「もう出さない」を端末に残す(iOS の `CoachMarkStore`)。
 * 変わると [state] が変わり、画面が描き直す。
 */
class CoachMarkStore(
    private val store: KeyValueStore,
    private val clock: () -> Long = System::currentTimeMillis,
    private val zone: () -> ZoneId = ZoneId::systemDefault,
) {
    data class State(
        val done: Set<CoachMark> = emptySet(),
        val optedOut: Boolean = false,
        val lastShown: CoachMarkSchedule.LastShown? = null,
        /** 設定の「もう一度見る」の後(保存しない): 1 日 1 つの間隔を空けずに続けて出す */
        val replaying: Boolean = false,
        /** 自動テスト用に出さない(保存しない) */
        val suppressed: Boolean = false,
    )

    private val _state = MutableStateFlow(read())
    val state: StateFlow<State> = _state.asStateFlow()

    /** いま出すもの(`eligible` = この場面で使える操作)。 */
    fun next(eligible: Set<CoachMark>, s: State = _state.value): CoachMark? {
        if (s.suppressed) return null
        return CoachMarkSchedule.next(eligible, s.done, s.optedOut, s.replaying, s.lastShown, clock(), zone())
    }

    fun handle(mark: CoachMark, event: CoachMarkEvent) {
        when (event) {
            CoachMarkEvent.SHOWN -> markShown(mark)
            CoachMarkEvent.DISMISSED -> finish(mark)
            CoachMarkEvent.NEVER_AGAIN -> {
                finish(mark)
                optOut()
            }
        }
    }

    /** 画面に出した時。同じものを出し直す時は時刻を変えない(1 日 1 つの数え方を崩さない)。 */
    fun markShown(mark: CoachMark) {
        if (_state.value.lastShown?.mark == mark) return
        val now = clock()
        store.putString(LAST_SHOWN_KEY, mark.key)
        store.putLong(LAST_SHOWN_AT_KEY, now)
        _state.value = _state.value.copy(lastShown = CoachMarkSchedule.LastShown(mark, now))
    }

    /** 実際にその操作をした時(吹き出しを出す前でも)。次から出さない。 */
    fun markPerformed(mark: CoachMark) = finish(mark)

    /** 「もう出さない」: 残りもすべて出さない。 */
    fun optOut() {
        store.putBoolean(OPT_OUT_KEY, true)
        _state.value = _state.value.copy(optedOut = true)
    }

    /** 設定の「操作のヒントをもう一度見る」: すべて未読に戻し、間隔を空けずに順に出す。 */
    fun replay() {
        clear()
        _state.value = read().copy(replaying = true)
    }

    /** 後片付け(#841・iOS の `LocalWipe.clearDefaults`)。次の人には未読から出し直す。 */
    fun forget() {
        clear()
        _state.value = read().copy(suppressed = _state.value.suppressed)
    }

    /** 自動のテスト・モック用(iOS の `METAMON_AI_CONSENT=granted` の時): FAB の上の吹き出しで画面を塞がない。 */
    fun suppress() {
        _state.value = _state.value.copy(suppressed = true)
    }

    private fun finish(mark: CoachMark) {
        val s = _state.value
        if (mark in s.done) return
        store.putBoolean(doneKey(mark), true)
        val done = s.done + mark
        _state.value = s.copy(done = done, replaying = s.replaying && !done.containsAll(CoachMark.entries))
    }

    private fun clear() {
        CoachMark.entries.forEach { store.remove(doneKey(it)) }
        store.remove(OPT_OUT_KEY)
        store.remove(LAST_SHOWN_KEY)
        store.remove(LAST_SHOWN_AT_KEY)
    }

    private fun read(): State {
        val mark = CoachMark.of(store.getString(LAST_SHOWN_KEY))
        val at = store.getLong(LAST_SHOWN_AT_KEY)
        return State(
            done = CoachMark.entries.filter { store.getBoolean(doneKey(it)) == true }.toSet(),
            optedOut = store.getBoolean(OPT_OUT_KEY) == true,
            lastShown = if (mark != null && at != null) CoachMarkSchedule.LastShown(mark, at) else null,
        )
    }

    companion object {
        /** SharedPreferences の名前 */
        const val PREFS = "metamon.coachMark"
        const val DONE_KEY_PREFIX = "done."
        const val OPT_OUT_KEY = "optOut"
        const val LAST_SHOWN_KEY = "lastShown"
        const val LAST_SHOWN_AT_KEY = "lastShownAt"

        fun doneKey(mark: CoachMark): String = DONE_KEY_PREFIX + mark.key
    }
}
