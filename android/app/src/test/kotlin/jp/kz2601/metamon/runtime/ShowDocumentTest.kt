package jp.kz2601.metamon.runtime

import jp.kz2601.metamon.dsl.AppDocument
import jp.kz2601.metamon.runtime.RuntimeFixture.runtime
import kotlinx.coroutines.test.runTest
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/** 生成の結果を当てたら、新しい DSL の entity の記録を読む(端末に残っている記録が一覧に出る)。 */
class ShowDocumentTest {
    private val generated = RuntimeFixture.dsl("warikan")

    @Test
    fun showLoadsRecordsOfTheNewDsl() = runTest {
        val rt = runtime(RuntimeFixture.dsl("initial"))
        rt.recordSync.save("test-app", "expenses", null, JsonObject(mapOf("title" to JsonPrimitive("ランチ"))))

        rt.show(AppDocument("test-app", 2, generated))

        assertEquals(1, rt.records("expenses").size)
        assertEquals(1, rt.recordsFlow.value["expenses"]?.size)
    }

    @Test
    fun showWithNothingStoredMarksEmptyInsteadOfLoadingForever() = runTest {
        val rt = runtime(RuntimeFixture.dsl("initial"))

        rt.show(AppDocument("test-app", 2, generated))

        // 圏外・ローカルも空 = 「読み込めませんでした」(未読の null のままにしない)
        assertTrue(rt.state.value.records.containsKey("expenses"))
        assertTrue("expenses" in rt.state.value.failed)
    }
}
