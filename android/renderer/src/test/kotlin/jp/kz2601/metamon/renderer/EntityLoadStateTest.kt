package jp.kz2601.metamon.renderer

import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.renderer.components.lists.LeafData
import jp.kz2601.metamon.renderer.components.lists.RecordsPlaceholder
import org.junit.Assert.assertEquals
import org.junit.Test

/** 一覧・表・時系列・グラフの 0 件の出し分け(#1116)。 */
class EntityLoadStateTest {
    private val known = setOf("task", "memo")
    private val none = emptyMap<String, List<Record>>()

    private fun of(records: Map<String, List<Record>> = none, loading: Set<String> = emptySet(), failed: Set<String> = emptySet()) =
        EntityLoadState.of("task", records, loading, failed, known)

    @Test
    fun beforeFirstLoadIsLoading() {
        assertEquals(EntityLoadState.LOADING, of())
        assertEquals(EntityLoadState.LOADING, of(loading = setOf("task")))
    }

    @Test
    fun loadedEmptyIsLoaded() {
        assertEquals(EntityLoadState.LOADED, of(records = mapOf("task" to emptyList())))
    }

    @Test
    fun backgroundRefreshOfLoadedEntityStaysLoaded() {
        // 空の案内と骨組みが行き来しない
        assertEquals(EntityLoadState.LOADED, of(records = mapOf("task" to emptyList()), loading = setOf("task")))
    }

    @Test
    fun failedIsUnavailableAndRetryIsLoading() {
        val records = mapOf("task" to emptyList<Record>())
        assertEquals(EntityLoadState.UNAVAILABLE, of(records = records, failed = setOf("task")))
        assertEquals(EntityLoadState.LOADING, of(records = records, loading = setOf("task"), failed = setOf("task")))
    }

    @Test
    fun unknownEntityIsLoaded() {
        assertEquals(EntityLoadState.LOADED, EntityLoadState.of("other", none, emptySet(), emptySet(), known))
    }

    @Test
    fun allCoversKnownEntities() {
        val all = EntityLoadState.all(mapOf("memo" to emptyList<Record>()), emptySet(), emptySet(), known)
        assertEquals(mapOf("task" to EntityLoadState.LOADING, "memo" to EntityLoadState.LOADED), all)
    }

    @Test
    fun placeholderPerState() {
        assertEquals(RecordsPlaceholder.SKELETON, RecordsPlaceholder.of(EntityLoadState.LOADING))
        assertEquals(RecordsPlaceholder.UNAVAILABLE, RecordsPlaceholder.of(EntityLoadState.UNAVAILABLE))
        assertEquals(RecordsPlaceholder.EMPTY, RecordsPlaceholder.of(EntityLoadState.LOADED))
    }

    @Test
    fun leafDataPicksPlaceholderForComponentEntity() {
        val states = mapOf("task" to EntityLoadState.LOADING, "memo" to EntityLoadState.UNAVAILABLE)
        val data = LeafData(NoopDSLRuntime, emptyMap(), loadStates = states)
        assertEquals(RecordsPlaceholder.SKELETON, data.placeholder("task"))
        assertEquals(RecordsPlaceholder.UNAVAILABLE, data.placeholder("memo"))
        assertEquals(RecordsPlaceholder.EMPTY, data.placeholder("other"))
        // entity の無い部品・状態を持たないホスト(プレビュー・適合テスト)は空の案内
        assertEquals(RecordsPlaceholder.EMPTY, data.placeholder(null))
        assertEquals(RecordsPlaceholder.EMPTY, LeafData(NoopDSLRuntime, emptyMap()).placeholder("task"))
    }
}
