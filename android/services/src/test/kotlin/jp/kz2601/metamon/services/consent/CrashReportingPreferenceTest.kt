package jp.kz2601.metamon.services.consent

import jp.kz2601.metamon.services.CrashReporter
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class CrashReportingPreferenceTest {
    private class Recording : CrashReporter {
        val enabled = mutableListOf<Boolean>()
        val users = mutableListOf<String?>()
        override fun setCollectionEnabled(enabled: Boolean) {
            this.enabled += enabled
        }

        override fun setUserId(uid: String?) {
            users += uid
        }
    }

    private val prefs = InMemoryKeyValueStore()

    @Test
    fun defaultsToOnAndAppliesOnActivate() {
        val pref = CrashReportingPreference(prefs)
        val reporter = Recording()
        assertTrue(pref.isEnabled)
        pref.activate(reporter)
        assertEquals(listOf(true), reporter.enabled)
    }

    @Test
    fun turningOffStopsCollectionAndDropsUid() {
        val pref = CrashReportingPreference(prefs)
        val reporter = Recording()
        pref.activate(reporter)
        pref.updateUser("u1")
        pref.setEnabled(false)
        assertEquals(false, reporter.enabled.last())
        assertEquals(null, reporter.users.last())
        pref.updateUser("u2") // オフの間は渡さない
        assertEquals(null, reporter.users.last())
        // 次の起動でもオフ
        val next = CrashReportingPreference(prefs)
        assertFalse(next.isEnabled)
        val r2 = Recording()
        next.activate(r2)
        assertEquals(listOf(false), r2.enabled)
    }

    @Test
    fun turningOnAgainPassesCurrentUid() {
        val pref = CrashReportingPreference(prefs)
        val reporter = Recording()
        pref.activate(reporter)
        pref.setEnabled(false)
        pref.updateUser("u1")
        pref.setEnabled(true)
        assertEquals(true, reporter.enabled.last())
        assertEquals("u1", reporter.users.last())
    }
}
