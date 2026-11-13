package jp.kz2601.metamon.desktop.os

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test
import java.util.Locale

/** Linux のドックの進み具合(#3011)。D-Bus が無い時に落ちない。 */
class LauncherEntryProgressTest {
    private class Recorder(var ok: Boolean = true) : CommandRunner {
        val calls = mutableListOf<List<String>>()
        override fun run(command: List<String>): Boolean {
            calls += command
            return ok
        }
    }

    @Test
    fun propertiesAreClampedAndLocaleFree() {
        assertEquals("{'progress-visible': <true>, 'progress': <0.250>}", LauncherEntryProgress.properties(0.25))
        assertEquals("{'progress-visible': <true>, 'progress': <1.000>}", LauncherEntryProgress.properties(7.0))
        assertEquals("{'progress-visible': <true>, 'progress': <0.000>}", LauncherEntryProgress.properties(-1.0))
        assertEquals("{'progress-visible': <false>, 'progress': <0.0>}", LauncherEntryProgress.properties(null))
        assertEquals("{'progress-visible': <false>, 'progress': <0.0>}", LauncherEntryProgress.properties(Double.NaN))
        val before = Locale.getDefault()
        try {
            Locale.setDefault(Locale.GERMANY)
            assertEquals("{'progress-visible': <true>, 'progress': <0.500>}", LauncherEntryProgress.properties(0.5))
        } finally {
            Locale.setDefault(before)
        }
    }

    @Test
    fun commandEmitsTheLauncherEntrySignal() {
        val c = LauncherEntryProgress.command("tailorit.desktop", "{}")
        assertEquals(listOf("gdbus", "emit", "--session"), c.take(3))
        assertTrue(c.containsAll(listOf("--object-path", "/", "--signal", "com.canonical.Unity.LauncherEntry.Update")))
        assertEquals(listOf("application://tailorit.desktop", "{}"), c.takeLast(2))
    }

    @Test
    fun sendsOnlyChangesAndClears() {
        val r = Recorder()
        val p = LauncherEntryProgress("tailorit.desktop", r, isLinux = true)
        p.update(0.5)
        p.update(0.5)
        p.update(0.6)
        p.clear()
        p.clear()
        assertEquals(3, r.calls.size)
        assertTrue(r.calls.last().last().contains("<false>"))
    }

    @Test
    fun failedSendIsRetriedNextTime() {
        val r = Recorder(ok = false)
        val p = LauncherEntryProgress("tailorit.desktop", r, isLinux = true)
        p.update(0.5)
        p.update(0.5)
        assertEquals(2, r.calls.size)
        r.ok = true
        p.update(0.5)
        p.update(0.5)
        assertEquals(3, r.calls.size)
    }

    @Test
    fun nonLinuxDoesNothing() {
        val r = Recorder()
        val p = LauncherEntryProgress("tailorit.desktop", r, isLinux = false)
        p.update(0.5)
        p.clear()
        assertTrue(r.calls.isEmpty())
    }

    @Test
    fun runnerThatThrowsInsideSystemRunnerReturnsFalse() {
        // gdbus が無い環境(コマンドが見つからない)でも落ちない
        assertFalse(SystemCommandRunner.run(listOf("tailorit-no-such-command-3011", "emit")))
    }

    @Test
    fun noDbusSessionDoesNotCrash() {
        // 実際のコマンドを動かす経路。D-Bus・gdbus が無くても(あっても)落ちずに戻る
        val p = LauncherEntryProgress("tailorit-test.desktop", SystemCommandRunner, isLinux = true)
        p.update(0.1)
        p.clear()
    }
}
