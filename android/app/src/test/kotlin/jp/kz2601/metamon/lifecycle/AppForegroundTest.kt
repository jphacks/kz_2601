package jp.kz2601.metamon.lifecycle

import kotlinx.coroutines.delay
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.advanceUntilIdle
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Test
import java.io.IOException

/** #891: 前面・背面の処理を 1 か所で決まった順に回す(iOS の scenePhase)。 */
class AppForegroundTest {
    private class Step(override val name: String, private val log: MutableList<String>, private val fail: Boolean = false) : ForegroundStep {
        override suspend fun foreground() {
            delay(10) // 前の段が終わる前に次へ進まないことを見る
            log += "$name+"
            if (fail) throw IOException("down")
        }

        override suspend fun background() {
            log += "$name-"
        }
    }

    private fun TestScope.coordinator(log: MutableList<String>, vararg failing: String): AppForeground =
        AppForeground(this, listOf("runtime", "push", "metrics").map { Step(it, log, fail = it in failing) })

    @Test
    fun foregroundRunsInOrderAndBackgroundInReverse() = runTest {
        val log = mutableListOf<String>()
        val fg = coordinator(log)
        fg.setForeground(false)
        fg.setForeground(true)
        advanceUntilIdle()
        assertEquals(listOf("metrics-", "push-", "runtime-", "runtime+", "push+", "metrics+"), log)
    }

    @Test
    fun sameDirectionTwiceRunsOnce() = runTest {
        val log = mutableListOf<String>()
        val fg = coordinator(log)
        fg.setForeground(true)
        fg.setForeground(true)
        advanceUntilIdle()
        assertEquals(listOf("runtime+", "push+", "metrics+"), log)
    }

    @Test
    fun firstStartAfterBuildingIsNotRepeated() = runTest {
        val log = mutableListOf<String>()
        val fg = coordinator(log)
        fg.markForeground()
        fg.setForeground(true)
        advanceUntilIdle()
        assertEquals("組み立てた時に済ませたので重ねない", emptyList<String>(), log)
    }

    @Test
    fun quickBackgroundWaitsForTheForegroundToFinish() = runTest {
        val log = mutableListOf<String>()
        val fg = coordinator(log)
        fg.markForeground()
        fg.setForeground(false)
        fg.setForeground(true)
        fg.setForeground(false)
        advanceUntilIdle()
        assertEquals(
            listOf("metrics-", "push-", "runtime-", "runtime+", "push+", "metrics+", "metrics-", "push-", "runtime-"),
            log,
        )
    }

    @Test
    fun aFailingStepDoesNotStopTheRest() = runTest {
        val log = mutableListOf<String>()
        val fg = coordinator(log, "push")
        fg.setForeground(true)
        advanceUntilIdle()
        assertEquals(listOf("runtime+", "push+", "metrics+"), log)
    }
}
