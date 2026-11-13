package jp.kz2601.metamon.desktop.onboarding

import jp.kz2601.metamon.desktop.store.JsonKeyValueStore
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder
import java.time.ZoneId

/** コーチマークの出す / 出さない(#3202・iOS の CoachMarkSchedule): 画面ごと・1 日 1 つ・閉じたら出さない・もう出さない・もう一度見る。 */
class CoachMarks3202Test {
    @get:Rule
    val tmp = TemporaryFolder()

    private val zone = ZoneId.of("Asia/Tokyo")
    private val day1 = 1_800_000_000_000L
    private val day2 = day1 + 86_400_000
    private val day3 = day1 + 86_400_000 * 2
    private var now = day1
    private val all = CoachMark.entries.toSet()

    private val file by lazy { tmp.root.resolve("settings.json") }

    private fun makeStore() = CoachMarkStore(JsonKeyValueStore(file), clock = { now }, zone = { zone })

    @Test
    fun coachMarkShownOnce() {
        val store = makeStore()
        val mark = store.next(CoachMarkPlace.HOME)
        assertEquals(CoachMark.COMMAND_PALETTE, mark)
        store.handle(mark!!, CoachMarkEvent.SHOWN)
        store.handle(mark, CoachMarkEvent.DISMISSED)
        now = day3
        assertEquals("閉じたものは二度と出さない", CoachMark.SHORTCUT_HELP, store.next(CoachMarkPlace.HOME))
        assertEquals("再起動しても同じ", CoachMark.SHORTCUT_HELP, makeStore().next(CoachMarkPlace.HOME))
    }

    @Test
    fun onlyMarksUsableOnThePlace() {
        val store = makeStore()
        assertEquals(setOf(CoachMark.DRAG_OUT), CoachMark.eligible(CoachMarkPlace.RECORD_LIST))
        assertEquals(CoachMark.DRAG_OUT, store.next(CoachMarkPlace.RECORD_LIST))
        assertFalse(CoachMark.DRAG_OUT in CoachMark.eligible(CoachMarkPlace.HOME))
        assertNull(store.next(emptySet()))
    }

    @Test
    fun shownButNotClosedComesBackAndOnlyOneNewPerDay() {
        val store = makeStore()
        store.markShown(CoachMark.COMMAND_PALETTE)
        assertEquals("閉じるまで同じものを出し続ける", CoachMark.COMMAND_PALETTE, makeStore().next(all))
        store.handle(CoachMark.COMMAND_PALETTE, CoachMarkEvent.DISMISSED)
        assertNull("初日に全部を出さない", store.next(all))
        now = day2
        assertEquals(CoachMark.SHORTCUT_HELP, store.next(all))
    }

    @Test
    fun reshowKeepsFirstShownTime() {
        val store = makeStore()
        store.markShown(CoachMark.COMMAND_PALETTE)
        now = day2
        store.markShown(CoachMark.COMMAND_PALETTE)
        assertEquals(day1, store.state.value.lastShown?.atMillis)
    }

    @Test
    fun performedBeforeShownIsSkipped() {
        val store = makeStore()
        store.markPerformed(CoachMark.COMMAND_PALETTE)
        assertEquals(CoachMark.SHORTCUT_HELP, store.next(CoachMarkPlace.APP))
    }

    @Test
    fun neverAgainStopsEverything() {
        val store = makeStore()
        store.handle(CoachMark.COMMAND_PALETTE, CoachMarkEvent.NEVER_AGAIN)
        now = day3
        assertNull(store.next(all))
        assertTrue(makeStore().state.value.optedOut)
    }

    @Test
    fun replayShowsAllInARowThenStops() {
        val store = makeStore()
        store.handle(CoachMark.COMMAND_PALETTE, CoachMarkEvent.NEVER_AGAIN)
        store.replay()
        for (expected in CoachMark.entries) {
            val mark = store.next(all)
            assertEquals(expected, mark)
            store.handle(mark!!, CoachMarkEvent.SHOWN)
            store.handle(mark, CoachMarkEvent.DISMISSED)
        }
        assertNull(store.next(all))
        assertFalse(store.state.value.replaying)
    }

    @Test
    fun forgetAndLaunchOverride() {
        val store = makeStore()
        store.handle(CoachMark.COMMAND_PALETTE, CoachMarkEvent.DISMISSED)
        store.forget()
        now = day2
        assertEquals(CoachMark.COMMAND_PALETTE, store.next(all))
        store.applyLaunchOverride("off")
        assertNull(store.next(all))
        store.applyLaunchOverride("reset")
        assertEquals(CoachMark.COMMAND_PALETTE, store.next(all))
        store.applyLaunchOverride(null)
        assertEquals(CoachMark.COMMAND_PALETTE, store.next(all))
    }
}
