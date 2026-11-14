package jp.kz2601.metamon.desktop.store

import jp.kz2601.metamon.services.Record
import jp.kz2601.metamon.services.local.LocalRecord
import jp.kz2601.metamon.services.local.RecordStore
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Test
import kotlin.coroutines.EmptyCoroutineContext

/** RecordStore.rows の並び: createdAt を時刻で比べる・同時刻は後から入った行が先・サーバーの同時刻の並びを保つ。 */
class RecordOrderTest {
    private val store = RecordStore(null, EmptyCoroutineContext)

    private fun row(id: String, createdAt: String?) = LocalRecord(appId = "a", entity = "e", recordId = id, createdAt = createdAt)

    private fun ids() = runBlocking { store.rows("a", "e").map { it.recordId } }

    @Test
    fun mixedFractionalPrecisionSortsByTime() = runBlocking {
        // 文字列で比べると ".5Z" > ".123456Z" > "Z" の順になり、時刻とずれる
        store.upsert(row("ms", "2026-11-12T10:00:00.123Z"))
        store.upsert(row("us", "2026-11-12T10:00:00.123456Z"))
        store.upsert(row("none", "2026-11-12T10:00:01Z"))
        store.upsert(row("half", "2026-11-12T10:00:00.5Z"))
        store.upsert(row("offset", "2026-11-12T19:00:02+09:00"))
        assertEquals(listOf("offset", "none", "half", "us", "ms"), ids())
    }

    @Test
    fun equalTimestampsNewestInsertFirstAndStable() = runBlocking {
        val t = "2026-11-12T10:00:00Z"
        listOf("local-c", "local-a", "local-b").forEach { store.upsert(row(it, t)) }
        assertEquals(listOf("local-b", "local-a", "local-c"), ids())
        // 更新しても位置は変わらない
        store.upsert(row("local-c", t).copy(updatedAt = "2026-11-12T11:00:00Z"))
        assertEquals(listOf("local-b", "local-a", "local-c"), ids())
        assertEquals(ids(), ids())
    }

    @Test
    fun missingOrBrokenCreatedAtGoesLast() = runBlocking {
        store.upsert(row("null", null))
        store.upsert(row("new", "2026-11-12T10:00:00Z"))
        store.upsert(row("broken", "yesterday"))
        assertEquals(listOf("new", "broken", "null"), ids())
    }

    @Test
    fun replaceKeepsServerOrderForTies() = runBlocking {
        val t = "2026-11-12T10:00:00Z"
        // サーバーは createdAt 降順・同時刻は id 降順で返す
        val server = listOf(
            Record(id = "z", createdAt = "2026-11-12T10:00:01Z"),
            Record(id = "c", createdAt = t),
            Record(id = "b", createdAt = t),
            Record(id = "a", createdAt = "2026-11-12T09:59:59.999999Z"),
        )
        store.replace("a", "e", server)
        assertEquals(listOf("z", "c", "b", "a"), ids())
        // その後に同時刻で作った行は先頭側
        store.upsert(row("local-new", t))
        assertEquals(listOf("z", "local-new", "c", "b", "a"), ids())
    }
}
