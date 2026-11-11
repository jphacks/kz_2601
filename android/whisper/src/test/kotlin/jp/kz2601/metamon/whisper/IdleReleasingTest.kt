package jp.kz2601.metamon.whisper

import kotlinx.coroutines.CompletableDeferred
import kotlinx.coroutines.awaitCancellation
import kotlinx.coroutines.launch
import kotlinx.coroutines.test.TestScope
import kotlinx.coroutines.test.advanceTimeBy
import kotlinx.coroutines.test.runCurrent
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class IdleReleasingTest {
    private class Model(val id: Int) {
        var released = false
    }

    private class Fixture(scope: TestScope) {
        var loads = 0
        val released = mutableListOf<Int>()
        val holder = IdleReleasing(
            scope = scope.backgroundScope,
            idleMillis = IDLE,
            load = { Model(++loads) },
            release = { it.released = true; released += it.id },
        )
    }

    @Test
    fun releasesAfterIdleSinceLastUse() = runTest {
        val f = Fixture(this)
        f.holder.use { }
        assertTrue(f.holder.isLoaded)
        advanceTimeBy(IDLE - 1)
        runCurrent()
        assertTrue(f.holder.isLoaded)
        advanceTimeBy(2)
        runCurrent()
        assertFalse(f.holder.isLoaded)
        assertEquals(listOf(1), f.released)
    }

    @Test
    fun useBeforeTimeoutResetsTheClockAndKeepsTheModel() = runTest {
        val f = Fixture(this)
        f.holder.use { }
        advanceTimeBy(IDLE - 10)
        f.holder.use { }
        advanceTimeBy(IDLE - 10)
        runCurrent()
        assertTrue(f.holder.isLoaded)
        assertEquals(1, f.loads)
        advanceTimeBy(20)
        runCurrent()
        assertFalse(f.holder.isLoaded)
    }

    @Test
    fun neverReleasesWhileInUse() = runTest {
        val f = Fixture(this)
        val gate = CompletableDeferred<Unit>()
        var seen: Model? = null
        val job = launch { f.holder.use { m -> seen = m; gate.await(); assertFalse(m.released) } }
        runCurrent()
        advanceTimeBy(IDLE * 5)
        runCurrent()
        assertTrue(f.holder.isLoaded)
        gate.complete(Unit)
        job.join()
        assertFalse(seen!!.released)
        advanceTimeBy(IDLE + 1)
        runCurrent()
        assertTrue(seen!!.released)
    }

    @Test
    fun reloadsAfterRelease() = runTest {
        val f = Fixture(this)
        f.holder.use { }
        advanceTimeBy(IDLE + 1)
        runCurrent()
        val id = f.holder.use { it.id }
        assertEquals(2, id)
        assertEquals(2, f.loads)
    }

    @Test
    fun releaseWhenIdleReleasesImmediatelyIfUnused() = runTest {
        val f = Fixture(this)
        f.holder.use { }
        f.holder.releaseWhenIdle()
        assertFalse(f.holder.isLoaded)
        assertEquals(listOf(1), f.released)
        // 待ちの時計は止まっている(2 回は手放さない)
        advanceTimeBy(IDLE * 2)
        runCurrent()
        assertEquals(listOf(1), f.released)
    }

    @Test
    fun releaseWhenIdleWaitsForTheCurrentUse() = runTest {
        val f = Fixture(this)
        val gate = CompletableDeferred<Unit>()
        val job = launch { f.holder.use { m -> gate.await(); assertFalse(m.released) } }
        runCurrent()
        f.holder.releaseWhenIdle()
        assertTrue(f.holder.isLoaded)
        gate.complete(Unit)
        job.join()
        assertFalse(f.holder.isLoaded)
        assertEquals(listOf(1), f.released)
    }

    @Test
    fun releaseWhenIdleWithNothingLoadedDoesNothing() = runTest {
        val f = Fixture(this)
        f.holder.releaseWhenIdle()
        assertEquals(0, f.loads)
        assertTrue(f.released.isEmpty())
    }

    @Test
    fun failedLoadLeavesNothingLoaded() = runTest {
        var attempts = 0
        val holder = IdleReleasing<Model>(backgroundScope, IDLE, load = { attempts++; error("no model") }, release = { })
        assertTrue(runCatching { holder.use { } }.isFailure)
        assertFalse(holder.isLoaded)
        assertEquals(1, attempts)
    }

    @Test
    fun cancelledUseStillStartsTheIdleClock() = runTest {
        val f = Fixture(this)
        val job = launch { f.holder.use { awaitCancellation() } }
        runCurrent()
        job.cancel()
        job.join()
        advanceTimeBy(IDLE + 1)
        runCurrent()
        assertFalse(f.holder.isLoaded)
        assertEquals(listOf(1), f.released)
    }

    private companion object {
        const val IDLE = 120_000L
    }
}
