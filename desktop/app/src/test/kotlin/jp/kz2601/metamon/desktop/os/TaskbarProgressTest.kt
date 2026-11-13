package jp.kz2601.metamon.desktop.os

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/** タスクバーの進み具合の計算(#3010)。Windows 以外では何もしない。 */
class TaskbarProgressTest {
    private fun countdown(remaining: Int, total: Int, running: Boolean = true) = TimerProgress(remaining, total, running)

    @Test
    fun countdownIsElapsedOverTotal() {
        assertEquals(TaskbarState.Normal(60, 300), TaskbarProgress.stateFor(listOf(countdown(240, 300))))
        assertEquals(TaskbarState.Normal(0, 300), TaskbarProgress.stateFor(listOf(countdown(300, 300))))
    }

    @Test
    fun stopwatchIsIndeterminate() {
        assertEquals(TaskbarState.Indeterminate, TaskbarProgress.stateFor(listOf(TimerProgress(42, null, true))))
    }

    @Test
    fun nothingRunningShowsNothing() {
        assertEquals(TaskbarState.None, TaskbarProgress.stateFor(emptyList()))
        assertEquals(TaskbarState.None, TaskbarProgress.stateFor(listOf(countdown(100, 300, running = false))))
    }

    @Test
    fun finishedOrBrokenTotalShowsNothing() {
        assertEquals(TaskbarState.None, TaskbarProgress.stateFor(listOf(countdown(0, 300))))
        assertEquals(TaskbarState.None, TaskbarProgress.stateFor(listOf(countdown(10, 0))))
    }

    @Test
    fun remainingOutOfRangeIsClamped() {
        assertEquals(TaskbarState.Normal(0, 300), TaskbarProgress.stateFor(listOf(countdown(999, 300))))
        assertEquals(TaskbarState.None, TaskbarProgress.stateFor(listOf(countdown(-5, 300))))
    }

    @Test
    fun firstRunningOneWins() {
        val timers = listOf(countdown(100, 300, running = false), countdown(30, 60), TimerProgress(5, null, true))
        assertEquals(TaskbarState.Normal(30, 60), TaskbarProgress.stateFor(timers))
    }

    @Test
    fun sendsOnlyChangesAndClears() {
        val shown = mutableListOf<TaskbarState>()
        val p = TaskbarProgress { shown += it }
        p.update(listOf(countdown(240, 300)))
        p.update(listOf(countdown(240, 300)))
        p.update(listOf(countdown(239, 300)))
        p.update(emptyList())
        p.update(emptyList())
        assertEquals(listOf(TaskbarState.Normal(60, 300), TaskbarState.Normal(61, 300), TaskbarState.None), shown)
    }

    @Test
    fun clearOnlyWhenSomethingIsShown() {
        val shown = mutableListOf<TaskbarState>()
        val p = TaskbarProgress { shown += it }
        p.clear()
        assertTrue(shown.isEmpty())
        p.update(listOf(countdown(10, 20)))
        p.clear()
        assertEquals(listOf(TaskbarState.Normal(10, 20), TaskbarState.None), shown)
    }

    @Test
    fun backendFailureDoesNotCrash() {
        val p = TaskbarProgress { error("COM が使えない") }
        p.update(listOf(countdown(10, 20)))
        p.clear()
    }

    @Test
    fun withoutBackendDoesNothing() {
        val p = TaskbarProgress(null)
        p.update(listOf(countdown(10, 20)))
        p.clear()
    }

    @Test
    fun nonWindowsHasNoBackendAndNoHandleNeeded() {
        if (System.getProperty("os.name").orEmpty().startsWith("Windows", ignoreCase = true)) return
        var asked = false
        val p = TaskbarProgress.forCurrentOs { asked = true; 1L }
        p.update(listOf(countdown(10, 20)))
        p.clear()
        assertTrue("Windows 以外ではウィンドウのハンドルも聞かない", !asked)
    }

    @Test
    fun windowsBackendWithoutHandleDoesNothing() {
        // ハンドルが無い時は COM にも触らない(どの OS でも落ちない)
        WindowsTaskbarBackend { null }.show(TaskbarState.Normal(1, 2))
    }
}
