package jp.kz2601.metamon.desktop.build

import jp.kz2601.metamon.desktop.build.PostBuildNotice.ACCOUNT_LINK
import jp.kz2601.metamon.desktop.build.PostBuildNotice.EDIT_HINT
import jp.kz2601.metamon.desktop.build.PostBuildNotice.PAYWALL
import jp.kz2601.metamon.desktop.build.PostBuildNotice.PUBLIC_DATA
import jp.kz2601.metamon.desktop.build.PostBuildNotice.RETENTION
import jp.kz2601.metamon.desktop.build.PostBuildNotice.SUMMARY
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class PostBuildNoticeTest {
    @Test
    fun nothingWaitingShowsNothing() {
        assertEquals(emptySet<PostBuildNotice>(), PostBuildNotice.pending(summary = false))
        assertNull(PostBuildNotice.current(emptySet()))
    }

    @Test
    fun eachFlagMapsToItsNotice() {
        assertEquals(setOf(SUMMARY), PostBuildNotice.pending(summary = true))
        assertEquals(setOf(RETENTION), PostBuildNotice.pending(summary = false, retention = true))
        assertEquals(setOf(EDIT_HINT), PostBuildNotice.pending(summary = false, editHint = true))
        assertEquals(setOf(PAYWALL), PostBuildNotice.pending(false, waiting = PostBuildWaiting(paywall = true)))
        assertEquals(setOf(PUBLIC_DATA), PostBuildNotice.pending(false, waiting = PostBuildWaiting(publicData = true)))
        assertEquals(setOf(ACCOUNT_LINK), PostBuildNotice.pending(false, waiting = PostBuildWaiting(accountLink = true)))
    }

    @Test
    fun smallestShownFirstOneAtATime() {
        val all = PostBuildNotice.pending(
            summary = true, retention = true, editHint = true,
            waiting = PostBuildWaiting(paywall = true, publicData = true, accountLink = true),
        )
        assertEquals(PostBuildNotice.entries.toSet(), all)
        val order = mutableListOf<PostBuildNotice>()
        var rest = all
        while (true) {
            val n = PostBuildNotice.current(rest) ?: break
            order += n
            rest = rest - n
        }
        assertEquals(listOf(PAYWALL, SUMMARY, PUBLIC_DATA, RETENTION, EDIT_HINT, ACCOUNT_LINK), order)
    }

    @Test
    fun pairsPickHigherPriority() {
        assertEquals(SUMMARY, PostBuildNotice.current(setOf(ACCOUNT_LINK, SUMMARY)))
        assertEquals(PAYWALL, PostBuildNotice.current(PostBuildNotice.pending(true, waiting = PostBuildWaiting(paywall = true))))
        assertEquals(EDIT_HINT, PostBuildNotice.current(PostBuildNotice.pending(false, editHint = true, waiting = PostBuildWaiting(accountLink = true))))
        assertEquals(RETENTION, PostBuildNotice.current(PostBuildNotice.pending(false, retention = true, editHint = true)))
    }
}
