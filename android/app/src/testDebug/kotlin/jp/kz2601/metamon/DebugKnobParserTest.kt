package jp.kz2601.metamon

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class DebugKnobParserTest {
    @Test
    fun emptyIsNone() {
        assertEquals(DebugKnobs.NONE, DebugKnobParser.parse(emptyMap()))
    }

    @Test
    fun parsesIosNames() {
        val k = DebugKnobParser.parse(
            mapOf(
                "METAMON_MOCK" to "1",
                "METAMON_MOCK_AI_DELAY" to "3.5",
                "METAMON_MOCK_SHARING" to "1",
                "METAMON_INITIAL_EXAMPLE" to "kakeibo",
                "METAMON_INITIAL_SCREEN" to "list",
                "METAMON_AUTO_TRANSCRIPT" to "家計簿アプリが欲しい",
                "METAMON_AUTO_NAV" to "home, list:sheet ,,detail:",
                "METAMON_AUTO_NAV_INTERVAL" to "1",
                "METAMON_E2E_OFFLINE_SECONDS" to "10",
                "METAMON_AUTO_FEEDBACK" to "1.5",
                "METAMON_ACCOUNT_LINK_PROMPTED" to "1",
            ),
        )
        assertTrue(k.mock)
        assertEquals(3.5, k.mockAiDelaySeconds!!, 0.0)
        assertTrue(k.mockSharing)
        assertEquals("kakeibo", k.initialExample)
        assertEquals("list", k.initialScreen)
        assertEquals("家計簿アプリが欲しい", k.autoTranscript)
        assertEquals(listOf("home", "list:sheet", "detail"), k.autoNav.map { it.toString() })
        assertEquals(1.0, k.autoNavIntervalSeconds, 0.0)
        assertEquals(10.0, k.offlineSeconds!!, 0.0)
        assertEquals(1.5, k.autoFeedbackSeconds!!, 0.0)
        assertNull(DebugKnobParser.parse(mapOf("METAMON_AUTO_FEEDBACK" to "-1")).autoFeedbackSeconds)
        assertEquals(true, k.accountLinkPrompted)
        assertEquals(false, DebugKnobParser.parse(mapOf("METAMON_ACCOUNT_LINK_PROMPTED" to "0")).accountLinkPrompted)
        assertNull(DebugKnobParser.parse(mapOf("METAMON_ACCOUNT_LINK_PROMPTED" to "yes")).accountLinkPrompted)
    }

    @Test
    fun brokenValuesFallBack() {
        val k = DebugKnobParser.parse(
            mapOf("METAMON_MOCK" to "true", "METAMON_MOCK_AI_DELAY" to "-1", "METAMON_E2E_OFFLINE_SECONDS" to "0", "METAMON_AUTO_NAV_INTERVAL" to "x"),
        )
        assertFalse(k.mock) // iOS と同じく "1" だけ
        assertNull(k.mockAiDelaySeconds)
        assertNull(k.offlineSeconds)
        assertEquals(2.0, k.autoNavIntervalSeconds, 0.0)
    }
}
