package jp.kz2601.metamon.desktop

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder
import java.io.File

class AccessBridgeAdviceTest2209 {
    @get:Rule val tmp = TemporaryFolder()

    @Test
    fun decideNeedsWindowsAndScreenReaderAndDisabledBridge() {
        for (win in listOf(true, false)) for (sr in listOf(true, false)) for (bridge in listOf(true, false)) {
            assertEquals("$win $sr $bridge", win && sr && !bridge, AccessBridgeAdvice.decide(win, sr, bridge))
        }
    }

    @Test
    fun propertiesParsing() {
        val on = "assistive_technologies=com.sun.java.accessibility.AccessBridge\n"
        assertTrue(AccessBridgeAdvice.bridgeEnabledIn(on))
        assertTrue(AccessBridgeAdvice.bridgeEnabledIn("assistive_technologies = a.B, com.sun.java.accessibility.AccessBridge"))
        assertFalse(AccessBridgeAdvice.bridgeEnabledIn("#$on"))
        assertFalse(AccessBridgeAdvice.bridgeEnabledIn("# comment\n#assistive_technologies=com.sun.java.accessibility.AccessBridge\n"))
        assertFalse(AccessBridgeAdvice.bridgeEnabledIn("assistive_technologies=other.Thing"))
        assertFalse(AccessBridgeAdvice.bridgeEnabledIn(""))
    }

    @Test
    fun bridgeEnabledReadsUserOrRuntimeFile() {
        val home = tmp.newFolder("home")
        val java = tmp.newFolder("java")
        File(java, "conf").mkdirs()
        assertFalse(AccessBridgeAdvice.bridgeEnabled(java.path, home.path))
        File(java, "conf/accessibility.properties").writeText("#assistive_technologies=com.sun.java.accessibility.AccessBridge\n")
        assertFalse(AccessBridgeAdvice.bridgeEnabled(java.path, home.path))
        File(home, ".accessibility.properties").writeText("assistive_technologies=com.sun.java.accessibility.AccessBridge\n")
        assertTrue(AccessBridgeAdvice.bridgeEnabled(java.path, home.path))
    }
}
