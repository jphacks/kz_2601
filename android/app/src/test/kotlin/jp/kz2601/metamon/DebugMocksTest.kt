package jp.kz2601.metamon

import jp.kz2601.metamon.onboarding.FirstRunTutorialStore
import jp.kz2601.metamon.runtime.RuntimeFixture
import jp.kz2601.metamon.services.MyUsage
import jp.kz2601.metamon.services.PlanUsageSource
import jp.kz2601.metamon.services.consent.AiConsentStore
import jp.kz2601.metamon.services.consent.InMemoryKeyValueStore
import jp.kz2601.metamon.services.mock.MockPlanUsageSource
import jp.kz2601.metamon.services.mock.MockSeed
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertSame
import org.junit.Assert.assertTrue
import org.junit.Test
import java.time.Instant

/** #1023・#1024: つまみを構成に当てる。release の [DebugKnobs.NONE] では何も差し替えない。 */
class DebugMocksTest {
    private val now = Instant.parse("2026-11-11T03:00:00Z")
    private val kakeibo = RuntimeFixture.dsl("kakeibo")

    private object LiveUsage : PlanUsageSource {
        override suspend fun myUsage(): MyUsage? = null
    }

    @Test
    fun noneKeepsTheRealServices() {
        val knobs = DebugKnobs.NONE
        assertEquals(MockSeed.demoRecords(now), DebugMocks.seed(kakeibo, knobs, now))
        assertSame(LiveUsage, DebugMocks.usage(knobs, LiveUsage))
        assertNull(DebugMocks.members(knobs))
        val consent = AiConsentStore(InMemoryKeyValueStore())
        val tutorial = FirstRunTutorialStore(InMemoryKeyValueStore())
        DebugMocks.applyAiConsent(knobs, consent, tutorial)
        assertFalse(consent.isGranted)
    }

    @Test
    fun usageKnobWinsEvenOnLiveAndMockHidesItOtherwise() {
        assertTrue(DebugMocks.usage(DebugKnobs(mockUsage = "apps:9/10"), LiveUsage) is MockPlanUsageSource)
        assertNull(DebugMocks.usage(DebugKnobs(mock = true), LiveUsage))
    }

    @Test
    fun membersNeedTheRoleKnob() {
        assertNotNull(DebugMocks.members(DebugKnobs(mockMembers = "owner")))
    }

    @Test
    fun seedDummyReplacesOnlyTheDslEntities() {
        val seed = DebugMocks.seed(kakeibo, DebugKnobs(seedDummy = 3), now)
        assertEquals(listOf("expense-1", "expense-2", "expense-3"), seed.getValue("expense").map { it.id })
        // 他の entity のデモの記録は残す
        assertEquals(MockSeed.demoRecords(now)["groceries"], seed["groceries"])
    }

    @Test
    fun seedAuthorsMixedAlternates() {
        val seed = DebugMocks.seed(kakeibo, DebugKnobs(seedDummy = 2, seedAuthorsMixed = true), now)
        assertEquals(listOf("mock-user", "mock00-other-uid"), seed.getValue("expense").map { it.author })
    }

    @Test
    fun aiConsentGrantedAndReset() {
        val consent = AiConsentStore(InMemoryKeyValueStore())
        val tutorial = FirstRunTutorialStore(InMemoryKeyValueStore())
        DebugMocks.applyAiConsent(DebugKnobs(aiConsent = DebugKnobs.AiConsentOverride.GRANTED), consent, tutorial)
        assertTrue(consent.isGranted)
        assertTrue(tutorial.hasSeenTutorial.value)
        DebugMocks.applyAiConsent(DebugKnobs(aiConsent = DebugKnobs.AiConsentOverride.RESET), consent, tutorial)
        assertFalse(consent.isGranted)
    }
}
