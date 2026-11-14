package jp.kz2601.metamon.desktop.store

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** 取り消せる削除: 戻す・時間切れ・アプリを消す を作り物の時計で確かめる。 */
class UndoableDeletionTest {
    private var clock = 1_000L
    private val sent = mutableListOf<DeletionTicket>()
    private val undo = UndoableDeletion(graceMillis = 5_000, now = { clock }, commit = { sent += it })

    @Test
    fun undoBeforeDeadlineSendsNothing() {
        val t = undo.delete("a", "memo", "r1")
        assertTrue(undo.isHidden("a", "memo", "r1"))
        clock += 4_999
        assertEquals(emptyList<DeletionTicket>(), undo.tick())
        assertTrue("時間内なら戻せる", undo.undo(t))
        assertFalse(undo.isHidden("a", "memo", "r1"))
        clock += 10_000
        undo.tick()
        assertEquals("戻したらサーバーには何も送らない", emptyList<DeletionTicket>(), sent)
        assertFalse("2 回目は戻せない", undo.undo(t))
    }

    @Test
    fun deadlineCommitsLikeNormalDelete() {
        val t1 = undo.delete("a", "memo", "r1")
        clock += 2_000
        val t2 = undo.delete("a", "memo", "r2")
        assertEquals(6_000L, undo.nextDeadline())
        clock += 3_000 // t1 だけ切れる
        assertEquals(listOf(t1), undo.tick())
        assertEquals(listOf(t1), sent)
        assertFalse("送った後は戻せない", undo.undo(t1))
        assertFalse(undo.isHidden("a", "memo", "r1"))
        assertTrue(undo.isHidden("a", "memo", "r2"))
        clock += 2_000
        undo.tick()
        assertEquals(listOf(t1, t2), sent)
        assertNull(undo.nextDeadline())
        assertTrue(undo.pendingDeletions.isEmpty())
    }

    @Test
    fun droppingAppDiscardsWithoutSending() {
        undo.delete("a", "memo", "r1")
        val other = undo.delete("b", "memo", "r1")
        assertEquals(1, undo.dropApp("a"))
        assertFalse(undo.isHidden("a", "memo", "r1"))
        assertEquals(listOf(other), undo.pendingDeletions)
        clock += 5_000
        undo.tick()
        assertEquals("消したアプリの分は送らない", listOf(other), sent)
    }

    @Test
    fun commitAllSendsWithoutWaiting() {
        val t = undo.delete("a", "memo", "r1")
        assertEquals(listOf(t), undo.commitAll())
        assertEquals(listOf(t), sent)
        assertFalse(undo.undo(t))
    }

    @Test
    fun dropAllForSignOut() {
        undo.delete("a", "memo", "r1")
        undo.delete("b", "memo", "r2")
        assertEquals(2, undo.drop())
        clock += 10_000
        undo.tick()
        assertTrue(sent.isEmpty())
    }
}
