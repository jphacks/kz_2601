package jp.kz2601.metamon.runtime

import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.Record
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Test

/** #1124: 通知に渡す記録。未読み込みは端末内 DB で埋め、読めなかった entity は端末内に無ければ外す(iOS の `recordSync.cached`)。 */
class NotificationRecordsTest {
    private fun rec(id: String) = Record(id, mapOf("name" to JSONValue.Str(id)))

    @Test
    fun fillsUnloadedEntitiesFromCache() = runTest {
        val asked = mutableListOf<String>()
        val out = NotificationRecords.fill(listOf("a", "b", "c"), mapOf("a" to listOf(rec("a1"))), emptySet()) { e ->
            asked += e
            if (e == "b") listOf(rec("b1")) else emptyList()
        }
        assertEquals(listOf("b", "c"), asked) // 読み込み済みは端末内 DB を読まない
        assertEquals(listOf("a1"), out["a"]!!.map { it.id })
        assertEquals(listOf("b1"), out["b"]!!.map { it.id })
        assertEquals("端末内にも無い entity は空として渡す", emptyList<Record>(), out["c"])
    }

    @Test
    fun failedEntityUsesCacheOrIsLeftOut() = runTest {
        val cache = mapOf("x" to listOf(rec("x1")))
        val out = NotificationRecords.fill(listOf("x", "y"), mapOf("x" to emptyList(), "y" to emptyList()), setOf("x", "y")) { cache[it].orEmpty() }
        assertEquals(listOf("x1"), out["x"]!!.map { it.id })
        assertFalse("読めず端末内にも無い entity は渡さない(条件を評価しない)", "y" in out)
    }
}
