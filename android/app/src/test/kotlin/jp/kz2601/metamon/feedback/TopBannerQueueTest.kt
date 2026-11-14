package jp.kz2601.metamon.feedback

import jp.kz2601.metamon.feedback.TopBannerQueue.Source.HOT_RELOAD
import jp.kz2601.metamon.feedback.TopBannerQueue.Source.SUMMARY
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class TopBannerQueueTest {
    private fun TopBannerQueue.of(summary: Any? = null, hotReload: Any? = null) =
        pick(mapOf(SUMMARY to summary, HOT_RELOAD to hotReload))

    @Test fun nothingShownWhenEmpty() {
        val pick = TopBannerQueue().of()
        assertNull(pick.shown)
        assertEquals(emptySet<TopBannerQueue.Source>(), pick.superseded)
    }

    @Test fun hotReloadAfterSummaryReplacesIt() {
        val q = TopBannerQueue()
        assertEquals(SUMMARY, q.of(summary = "a").shown)
        val pick = q.of(summary = "a", hotReload = 1L)
        assertEquals(HOT_RELOAD, pick.shown)
        assertEquals(setOf(SUMMARY), pick.superseded)
    }

    @Test fun summaryAfterHotReloadReplacesIt() {
        val q = TopBannerQueue()
        q.of(hotReload = 1L)
        val pick = q.of(summary = "a", hotReload = 1L)
        assertEquals(SUMMARY, pick.shown)
        assertEquals(setOf(HOT_RELOAD), pick.superseded)
    }

    @Test fun summaryWinsWhenBothArriveTogether() {
        val pick = TopBannerQueue().of(summary = "a", hotReload = 1L)
        assertEquals(SUMMARY, pick.shown)
        assertEquals(setOf(HOT_RELOAD), pick.superseded)
    }

    @Test fun samePickIsStableAcrossRecompositions() {
        val q = TopBannerQueue()
        q.of(summary = "a")
        q.of(summary = "a", hotReload = 1L)
        repeat(3) { assertEquals(HOT_RELOAD, q.of(summary = "a", hotReload = 1L).shown) }
    }

    @Test fun newHotReloadRoundReplacesTheShownSummaryAgain() {
        val q = TopBannerQueue()
        q.of(hotReload = 1L)
        assertEquals(SUMMARY, q.of(summary = "a", hotReload = 1L).shown)
        assertEquals(HOT_RELOAD, q.of(summary = "a", hotReload = 2L).shown)
    }

    @Test fun newSummaryKeyCountsAsNewArrival() {
        val q = TopBannerQueue()
        q.of(summary = "a")
        q.of(summary = "a", hotReload = 1L)
        assertEquals(SUMMARY, q.of(summary = "b", hotReload = 1L).shown)
    }

    @Test fun dismissedSourceReappearingIsNew() {
        val q = TopBannerQueue()
        q.of(summary = "a", hotReload = 1L)
        q.of(hotReload = 1L) // 要約を閉じた
        assertEquals(SUMMARY, q.of(summary = "a", hotReload = 1L).shown)
    }

    @Test fun afterSupersededIsClosedOnlyTheNewOneRemains() {
        val q = TopBannerQueue()
        q.of(summary = "a")
        q.of(summary = "a", hotReload = 1L)
        val pick = q.of(hotReload = 1L)
        assertEquals(HOT_RELOAD, pick.shown)
        assertEquals(emptySet<TopBannerQueue.Source>(), pick.superseded)
    }

    @Test fun timingMatchesSummaryRules() {
        assertEquals(3_000L, TopBannerTiming.baseMillis("最新の内容に更新しました", hasAction = false))
        assertEquals(5_000L, TopBannerTiming.baseMillis("あ".repeat(100), hasAction = false))
        assertEquals(4_000L, TopBannerTiming.baseMillis("あ".repeat(48), hasAction = false))
        assertEquals(6_000L, TopBannerTiming.baseMillis("短い", hasAction = true))
    }
}
