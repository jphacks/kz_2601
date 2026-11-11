package jp.kz2601.metamon.renderer

import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.renderer.components.lists.ListRowMenu
import jp.kz2601.metamon.renderer.components.social.ModerationHost
import org.junit.Assert.assertEquals
import org.junit.Test

/** #1196: list の行の長押しメニュー(iOS の ListLeaf の contextMenu = 削除 + ModerationMenuItems)。 */
class ListRowMenuTest {
    private class FakeModeration(private val reportable: Set<String>) : ModerationHost {
        override fun canReport(entity: String, record: Record) = record.id in reportable
        override fun reportRecord(entity: String, record: Record) = Unit
        override fun blockAuthor(entity: String, record: Record) = Unit
    }

    private val host = FakeModeration(setOf("others"))

    @Test
    fun othersSharedRecordGetsReportAndBlock() {
        assertEquals(
            listOf(ListRowMenu.Item.REPORT, ListRowMenu.Item.BLOCK),
            ListRowMenu.items(deletable = false, moderation = host, entity = "posts", record = Record("others")),
        )
    }

    @Test
    fun ownRecordGetsOnlyDelete() {
        assertEquals(listOf(ListRowMenu.Item.DELETE), ListRowMenu.items(true, host, "posts", Record("mine")))
        assertEquals(emptyList<ListRowMenu.Item>(), ListRowMenu.items(false, host, "posts", Record("mine")))
    }

    @Test
    fun deleteComesFirstLikeIos() {
        assertEquals(
            listOf(ListRowMenu.Item.DELETE, ListRowMenu.Item.REPORT, ListRowMenu.Item.BLOCK),
            ListRowMenu.items(true, host, "posts", Record("others")),
        )
    }

    @Test
    fun runtimeWithoutModerationShowsNoReport() {
        assertEquals(emptyList<ListRowMenu.Item>(), ListRowMenu.items(false, null, "posts", Record("others")))
    }
}
