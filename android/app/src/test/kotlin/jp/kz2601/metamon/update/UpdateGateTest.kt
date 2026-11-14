package jp.kz2601.metamon.update

import jp.kz2601.metamon.services.consent.InMemoryKeyValueStore
import jp.kz2601.metamon.services.update.ClientConfig
import jp.kz2601.metamon.services.update.NoticeDismissals
import jp.kz2601.metamon.services.update.ServiceNotice
import jp.kz2601.metamon.services.update.UpdateChecker
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class UpdateGateTest {
    private val notice = ServiceNotice(id = "n1", ja = "止めています")

    private fun gate(config: ClientConfig, build: String = "5", store: InMemoryKeyValueStore = InMemoryKeyValueStore()) =
        UpdateGate(UpdateChecker({ config }, build), NoticeDismissals(store))

    @Test
    fun dismissedNoticeStaysHiddenAcrossLaunches() = runBlocking {
        val store = InMemoryKeyValueStore()
        val first = gate(ClientConfig(notice = notice), store = store)
        first.check()
        assertEquals(notice, first.visibleNotice())
        first.dismissNotice()
        assertNull(first.visibleNotice())

        val next = gate(ClientConfig(notice = notice), store = store)
        next.check()
        assertNull(next.visibleNotice())
        // 新しい id は出す
        val changed = gate(ClientConfig(notice = notice.copy(id = "n2")), store = store)
        changed.check()
        assertEquals("n2", changed.visibleNotice()?.id)
    }

    @Test
    fun blockingScreenHidesTheBannerAndTheRecommendation() = runBlocking {
        val g = gate(ClientConfig(minAndroidBuild = 6, latestAndroidBuild = 9, notice = notice))
        g.check()
        assertTrue(g.state.value.isBlocking)
        assertFalse(g.state.value.showsRecommended)
        assertNull(g.visibleNotice())
    }

    @Test
    fun recommendationIsShownOncePerLatest() = runBlocking {
        val g = gate(ClientConfig(latestAndroidBuild = 9))
        g.check()
        assertTrue(g.state.value.showsRecommended)
        g.dismissRecommended()
        g.check()
        assertFalse(g.state.value.showsRecommended)
    }

    @Test
    fun withoutAndroidValuesNothingIsEnforced() = runBlocking {
        val g = gate(ClientConfig())
        g.check()
        assertFalse(g.state.value.isBlocking)
        assertFalse(g.state.value.showsRecommended)
        // モック(checker 無し)も何もしない
        val disabled = UpdateGate.disabled(InMemoryKeyValueStore())
        disabled.check()
        assertEquals(UpdateGate.State(), disabled.state.value)
    }

    @Test
    fun storeCandidatesTryMarketThenHttps() {
        assertEquals(
            listOf("market://details?id=jp.kz2601.metamon", "https://play.google.com/store/apps/details?id=jp.kz2601.metamon"),
            PlayStore.candidates("jp.kz2601.metamon", null),
        )
        assertEquals("https://example.com/app", PlayStore.candidates("x", "https://example.com/app").first())
    }
}
