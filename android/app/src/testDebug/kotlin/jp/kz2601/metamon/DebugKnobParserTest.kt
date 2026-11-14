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

    @Test
    fun parsesFeatureMockKnobs() {
        val k = DebugKnobParser.parse(
            mapOf(
                "METAMON_MOCK_NEW_ITEMS" to "2",
                "METAMON_MOCK_USAGE" to "apps:9/10,pro",
                "METAMON_MOCK_MEMBERS" to "member",
                "METAMON_MOCK_MEMBERS_FAIL" to "1",
                "METAMON_AI_CONSENT" to "granted",
                "METAMON_SEED_DUMMY" to "6",
                "METAMON_SEED_STEP_HOURS" to "24",
                "METAMON_SEED_AUTHORS" to "mixed",
            ),
        )
        assertEquals(2, k.mockNewItems)
        assertEquals("apps:9/10,pro", k.mockUsage)
        assertEquals("member", k.mockMembers)
        assertTrue(k.mockMembersFail)
        assertEquals(DebugKnobs.AiConsentOverride.GRANTED, k.aiConsent)
        assertEquals(6, k.seedDummy)
        assertEquals(24.0, k.seedStepHours, 0.0)
        assertTrue(k.seedAuthorsMixed)
        assertEquals(DebugKnobs.AiConsentOverride.RESET, DebugKnobParser.parse(mapOf("METAMON_AI_CONSENT" to "reset")).aiConsent)
    }

    @Test
    fun featureMockKnobsFallBackLikeIos() {
        val k = DebugKnobParser.parse(
            mapOf(
                "METAMON_MOCK_NEW_ITEMS" to "x",
                "METAMON_MOCK_MEMBERS" to "admin",
                "METAMON_MOCK_MEMBERS_FAIL" to "true",
                "METAMON_AI_CONSENT" to "yes",
                "METAMON_SEED_DUMMY" to "many",
                "METAMON_SEED_STEP_HOURS" to "-5",
                "METAMON_SEED_AUTHORS" to "other",
            ),
        )
        assertNull(k.mockNewItems)
        assertNull(k.mockMembers)
        assertFalse(k.mockMembersFail)
        assertNull(k.aiConsent)
        assertEquals(4, k.seedDummy) // 数字でなければ 4 件
        assertEquals(0.0, k.seedStepHours, 0.0) // 0〜1 年に丸める
        assertFalse(k.seedAuthorsMixed)
        assertEquals(10, DebugKnobParser.parse(mapOf("METAMON_SEED_DUMMY" to "99")).seedDummy)
        assertEquals(1, DebugKnobParser.parse(mapOf("METAMON_SEED_DUMMY" to "0")).seedDummy)
        assertNull(DebugKnobs.NONE.seedDummy)
        assertEquals(1.0, DebugKnobs.NONE.seedStepHours, 0.0)
    }
}
