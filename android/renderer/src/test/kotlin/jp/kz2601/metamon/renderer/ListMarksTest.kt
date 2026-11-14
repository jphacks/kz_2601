package jp.kz2601.metamon.renderer

import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.Mark
import jp.kz2601.metamon.dsl.records.MarkState
import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.renderer.components.lists.ListMarks
import jp.kz2601.metamon.renderer.components.lists.PublicDataNotice
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertNull
import org.junit.Test

/** #1115: 一覧の行の印(いいね・#255)と公開の表示(#253)。iOS の ListLeaf.toggleMark・PublicDataNoticeTests と同じ。 */
class ListMarksTest {
    private val like = Mark("like", "heart", "いいね")
    private val shared = Entity("posts", scope = "shared", marks = listOf(like))

    @Test
    fun markShowsOnlyOnSharedEntitiesThatDeclareIt() {
        assertEquals(like, ListMarks.mark("like", shared))
        assertEquals(like, ListMarks.mark("like", shared.copy(scope = "public")))
        assertNull(ListMarks.mark("like", shared.copy(scope = null)))
        assertNull(ListMarks.mark("like", shared.copy(scope = "private")))
        assertNull(ListMarks.mark("save", shared))
        assertNull(ListMarks.mark(null, shared))
        assertNull(ListMarks.mark("like", null))
    }

    @Test
    fun tapShowsTheNextStateAtOnceAndSuccessSettlesToTheStore() {
        val overrides = mutableMapOf<String, MarkState>()
        val before = Record("r1", marks = mapOf("like" to MarkState(2, false)))
        val sent = ListMarks.tap(overrides, before, "like")
        assertEquals(MarkState(3, true), sent)
        assertEquals(MarkState(3, true), ListMarks.shown(overrides, before, "like"))
        // 成功: ストアが新しい値になってから上書きを外す
        val after = before.copy(marks = mapOf("like" to MarkState(3, true)))
        ListMarks.settle(overrides, "r1", sent)
        assertEquals(emptyMap<String, MarkState>(), overrides)
        assertEquals(MarkState(3, true), ListMarks.shown(overrides, after, "like"))
    }

    @Test
    fun failureRollsBackToTheStoredState() {
        val overrides = mutableMapOf<String, MarkState>()
        val row = Record("r1", marks = mapOf("like" to MarkState(1, true)))
        val sent = ListMarks.tap(overrides, row, "like")
        assertEquals(MarkState(0, false), ListMarks.shown(overrides, row, "like"))
        // 失敗: ストアは元の値のまま → 上書きを外すと元に戻る
        ListMarks.settle(overrides, "r1", sent)
        assertEquals(MarkState(1, true), ListMarks.shown(overrides, row, "like"))
    }

    @Test
    fun rapidSecondTapKeepsItsOwnOverride() {
        val overrides = mutableMapOf<String, MarkState>()
        val row = Record("r1")
        val first = ListMarks.tap(overrides, row, "like")
        val second = ListMarks.tap(overrides, row, "like")
        assertEquals(MarkState(1, true), first)
        assertEquals(MarkState(0, false), second)
        ListMarks.settle(overrides, "r1", first) // 1 回目の返事では外さない
        assertEquals(MarkState(0, false), ListMarks.shown(overrides, row, "like"))
        ListMarks.settle(overrides, "r1", second)
        assertEquals(MarkState(0, false), ListMarks.shown(overrides, row, "like"))
    }

    @Test
    fun countNeverGoesNegative() {
        val overrides = mutableMapOf<String, MarkState>()
        val broken = Record("r1", marks = mapOf("like" to MarkState(0, true)))
        assertEquals(MarkState(0, false), ListMarks.tap(overrides, broken, "like"))
    }

    @Test
    fun symbols() {
        assertEquals("heart.fill", ListMarks.symbol("heart", on = true))
        assertEquals("heart", ListMarks.symbol("heart", on = false))
        assertEquals("heart", ListMarks.symbol("unknown", on = false))
        assertEquals("hand.thumbsup", ListMarks.symbol("thumbsup", on = true))
        assertEquals("checkmark.circle", ListMarks.symbol("check", on = true))
        assertEquals("star", ListMarks.symbol("star", on = true))
    }

    @Test
    fun publicNoticeOnlyForSharedOrPublic() {
        assertEquals(R.string.lists_public_notice, PublicDataNotice.text("public"))
        assertEquals(R.string.lists_shared_notice, PublicDataNotice.text("shared"))
        assertNotEquals(PublicDataNotice.text("public"), PublicDataNotice.text("shared"))
        assertNull(PublicDataNotice.text("private"))
        assertNull(PublicDataNotice.text(null))
        assertNull(PublicDataNotice.text("everyone"))
        assertEquals("globe", PublicDataNotice.icon("public"))
        assertEquals("person.2", PublicDataNotice.icon("shared"))
    }
}
