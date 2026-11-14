package jp.kz2601.metamon.onboarding

import jp.kz2601.metamon.services.consent.InMemoryKeyValueStore
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.time.ZoneId

/** 気づきにくい操作の吹き出し(#900・iOS の CoachMarkTests): 順番・1 日 1 つ・閉じた / 操作したら出さない・もう出さない・もう一度見る・後片付け。 */
class CoachMarkTest {
    private val prefs = InMemoryKeyValueStore()
    private val zone = ZoneId.of("Asia/Tokyo")
    private val day1 = 1_800_000_000_000L
    private val day1Later = day1 + 60_000
    private val day2 = day1 + 86_400_000
    private val day3 = day1 + 86_400_000 * 2
    private var now = day1
    private val all = CoachMark.entries.toSet()

    private fun makeStore() = CoachMarkStore(prefs, clock = { now }, zone = { zone })

    @Test
    fun showsInOrderOnlyWhereUsable() {
        val store = makeStore()
        assertEquals(CoachMark.FAB_LONG_PRESS, store.next(all))
        assertEquals(CoachMark.FAB_DRAG, store.next(setOf(CoachMark.FAB_DRAG)))
        assertNull(store.next(emptySet()))
    }

    @Test
    fun shownMarkStaysUntilClosedButOnlyOnePerDay() {
        val store = makeStore()
        store.handle(CoachMark.FAB_LONG_PRESS, CoachMarkEvent.SHOWN)
        now = day1Later
        assertEquals("閉じるまで同じものを出し続ける", CoachMark.FAB_LONG_PRESS, store.next(all))
        store.handle(CoachMark.FAB_LONG_PRESS, CoachMarkEvent.DISMISSED)
        assertNull("初日に全部を出さない", store.next(all))
        now = day2
        assertEquals(CoachMark.FAB_DRAG, store.next(all))
    }

    @Test
    fun dismissedNeverShownAgainAndPersisted() {
        val store = makeStore()
        store.handle(CoachMark.FAB_LONG_PRESS, CoachMarkEvent.SHOWN)
        store.handle(CoachMark.FAB_LONG_PRESS, CoachMarkEvent.DISMISSED)
        now = day3
        assertNull(store.next(setOf(CoachMark.FAB_LONG_PRESS)))
        assertNull("ヒントごとに保存される", makeStore().next(setOf(CoachMark.FAB_LONG_PRESS)))
    }

    @Test
    fun performedBeforeShownIsSkipped() {
        val store = makeStore()
        store.markPerformed(CoachMark.FAB_LONG_PRESS)
        assertEquals(CoachMark.FAB_DRAG, store.next(CoachMark.FAB))
        store.markShown(CoachMark.FAB_DRAG)
        store.markPerformed(CoachMark.FAB_DRAG)
        now = day1Later
        assertNull("出ている吹き出しも操作したら消える", store.next(CoachMark.FAB))
    }

    @Test
    fun neverAgainStopsEverythingAndPersists() {
        val store = makeStore()
        store.handle(CoachMark.FAB_LONG_PRESS, CoachMarkEvent.SHOWN)
        store.handle(CoachMark.FAB_LONG_PRESS, CoachMarkEvent.NEVER_AGAIN)
        now = day3
        assertNull(store.next(all))
        assertNull(makeStore().next(all))
    }

    @Test
    fun unclosedMarkComesBackAfterRelaunch() {
        makeStore().markShown(CoachMark.FAB_DRAG)
        now = day1Later
        val reloaded = makeStore()
        assertEquals(CoachMark.FAB_DRAG, reloaded.next(all))
        assertNull("同じ日は別の場面でも新しいものを出さない", reloaded.next(setOf(CoachMark.FAB_LONG_PRESS)))
        now = day2
        assertEquals(CoachMark.FAB_LONG_PRESS, reloaded.next(setOf(CoachMark.FAB_LONG_PRESS)))
    }

    @Test
    fun markShownTwiceKeepsFirstTime() {
        val store = makeStore()
        store.markShown(CoachMark.FAB_LONG_PRESS)
        now = day2
        store.markShown(CoachMark.FAB_LONG_PRESS)
        assertEquals(day1, store.state.value.lastShown?.atMillis)
    }

    @Test
    fun replayResetsAndShowsBackToBack() {
        val store = makeStore()
        CoachMark.entries.forEach(store::markPerformed)
        store.optOut()
        store.suppress()
        store.replay()
        assertFalse(store.state.value.optedOut)
        assertEquals(CoachMark.FAB_LONG_PRESS, store.next(all))
        store.handle(CoachMark.FAB_LONG_PRESS, CoachMarkEvent.SHOWN)
        store.handle(CoachMark.FAB_LONG_PRESS, CoachMarkEvent.DISMISSED)
        now = day1Later
        assertEquals("もう一度見る間は同じ日でも続けて出す", CoachMark.FAB_DRAG, store.next(all))
        store.handle(CoachMark.FAB_DRAG, CoachMarkEvent.DISMISSED)
        assertFalse("全部見たら普段の間隔に戻る", store.state.value.replaying)
    }

    @Test
    fun suppressedShowsNothing() {
        val store = makeStore()
        store.suppress()
        assertNull(store.next(all))
    }

    @Test
    fun forgetClearsForNextPerson() {
        val store = makeStore()
        store.handle(CoachMark.FAB_LONG_PRESS, CoachMarkEvent.NEVER_AGAIN)
        store.forget()
        assertTrue(store.state.value.done.isEmpty())
        assertEquals(CoachMark.FAB_LONG_PRESS, store.next(all))
        assertEquals("保存も消えている", CoachMark.FAB_LONG_PRESS, makeStore().next(all))
    }
}
