package jp.kz2601.metamon.runtime

import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.renderer.EntityLoadState
import jp.kz2601.metamon.runtime.RuntimeFixture.runtime
import kotlinx.coroutines.test.runTest
import org.junit.Assert.assertEquals
import org.junit.Test

/** AppRuntime が描画器に渡す entity ごとの読み込み状態(#1116)。サーバーには届かない(圏外)。 */
class EntityLoadStateRuntimeTest {
    private val dsl = RuntimeFixture.dsl("kakeibo")
    private val entity = dsl.entities.first().id

    @Test
    fun beforeLoadIsLoadingThenUnavailableWhenServerAndLocalAreEmpty() = runTest {
        val rt = runtime(dsl)
        // 起動の読み込みより前: 部品は骨組みを出す
        assertEquals(EntityLoadState.LOADING, rt.loadStatesFlow.value[entity])
        rt.load(setOf(entity))
        assertEquals(EntityLoadState.UNAVAILABLE, rt.loadStatesFlow.value[entity])
    }

    @Test
    fun retryClearsUnavailableOnlyWhenReadable() = runTest {
        val rt = runtime(dsl)
        rt.load(setOf(entity))
        assertEquals(EntityLoadState.UNAVAILABLE, rt.loadStatesFlow.value[entity])
        // まだ圏外: 再試行しても取得失敗のまま(読み直し中だけ LOADING)
        rt.retryRecords(setOf(entity))
        assertEquals(EntityLoadState.UNAVAILABLE, rt.loadStatesFlow.value[entity])
    }

    @Test
    fun localRecordsMakeItLoadedEvenWhenServerFails() = runTest {
        val rt = runtime(dsl)
        val field = dsl.entities.first().fields.first().id
        rt.createRecord(entity, mapOf(field to JSONValue.Str("x")))
        rt.retryRecords(setOf(entity))
        assertEquals(EntityLoadState.LOADED, rt.loadStatesFlow.value[entity])
    }
}
