package jp.kz2601.metamon.renderer

import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.dsl.Show
import jp.kz2601.metamon.dsl.records.InterestState
import jp.kz2601.metamon.dsl.records.MatchBox
import jp.kz2601.metamon.dsl.records.MatchInterest
import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.renderer.components.social.MatchListRows
import jp.kz2601.metamon.renderer.components.social.ReportReasonOption
import jp.kz2601.metamon.renderer.registry.ComponentRegistry
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** 仲間探し(#877): 登録・候補の並び・空の形・行の操作。 */
class MatchListLeafTest {
    @Test
    fun matchListIsRegistered() {
        assertTrue(ComponentType.MATCH_LIST.wire in ComponentRegistry.standard.registeredTypes)
    }

    @Test
    fun showMapsToABox() {
        assertEquals(MatchBox.INCOMING, MatchBox.of(Show.INCOMING))
        assertEquals(MatchBox.MATCHES, MatchBox.of(Show.MATCHES))
        assertNull(MatchBox.of(Show.UNKNOWN)) // 知らない値は出さない
        assertNull(MatchBox.of(null))
    }

    @Test
    fun rowsKeepServerOrderAndLookUpProfiles() {
        val items = listOf(
            MatchInterest("u2", "p2"),
            MatchInterest("u1", "p1"),
            MatchInterest("u3", "gone"), // プロフィールが消された人も操作だけ出す
            MatchInterest("u2", "p2"), // 同じ人は 1 行
        )
        val rows = MatchListRows.rows(items, listOf(Record("p1"), Record("p2")))
        assertEquals(listOf("u2", "u1", "u3"), rows.map { it.item.uid })
        assertEquals(listOf("p2", "p1", null), rows.map { it.profile?.id })
        assertTrue(MatchListRows.rows(null, emptyList()).isEmpty())
    }

    @Test
    fun emptyStateIsLoadingFailedOrEmpty() {
        assertEquals(MatchListRows.EmptyState.EMPTY, MatchListRows.emptyState(hasHost = false, items = null, failed = false))
        assertEquals(MatchListRows.EmptyState.LOADING, MatchListRows.emptyState(hasHost = true, items = null, failed = false))
        assertEquals(MatchListRows.EmptyState.FAILED, MatchListRows.emptyState(hasHost = true, items = null, failed = true))
        assertEquals(MatchListRows.EmptyState.EMPTY, MatchListRows.emptyState(hasHost = true, items = emptyList(), failed = true))
    }

    @Test
    fun rowActionsFollowTheBox() {
        assertEquals(listOf(MatchListRows.RowAction.ACCEPT, MatchListRows.RowAction.DECLINE), MatchListRows.actions(MatchBox.INCOMING))
        assertEquals(listOf(MatchListRows.RowAction.WITHDRAW), MatchListRows.actions(MatchBox.OUTGOING))
        assertEquals(listOf(MatchListRows.RowAction.UNMATCH), MatchListRows.actions(MatchBox.MATCHES))
    }

    @Test
    fun onlyNoneAndIncomingAreTappable() {
        assertTrue(InterestState.NONE.tappable)
        assertTrue(InterestState.INCOMING.tappable)
        assertFalse(InterestState.SENT.tappable)
        assertFalse(InterestState.MATCHED.tappable)
    }

    @Test
    fun reportReasonsAreTheApiValuesInIosOrder() {
        assertEquals(
            listOf("spam", "sexual", "violence", "harassment", "illegal", "privacy", "other"),
            ReportReasonOption.entries.map { it.wire },
        )
    }
}
