package jp.kz2601.metamon.services.feeds

import jp.kz2601.metamon.dsl.feed.FeedQuery
import jp.kz2601.metamon.dsl.feed.FeedResult
import jp.kz2601.metamon.services.local.AtomicFile
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withContext
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.longOrNull
import kotlinx.serialization.json.put
import kotlinx.serialization.json.buildJsonObject
import java.io.File
import java.security.MessageDigest
import kotlin.coroutines.CoroutineContext

/**
 * 同じ問い合わせを覚えておく(iOS の `FeedCache`・10 分)。画面を行き来するたびに取りに行かない。
 * iOS はメモリだけだが、Android は `DocumentCache` と同じ流儀で端末にも置き、オフラインの起動でも前の行を出す。
 * - [ttlMillis] 以内は取りに行かない(`force` は取り直す)
 * - 取れなかった時、[staleLimitMillis](サーバーの stale と同じ 24 時間)以内の行があれば `stale = true` で返す
 *
 * @param directory 置き場(例 `filesDir/metamon-feeds`)。null ならメモリだけ(テスト・モック)
 */
class FeedCache(
    private val directory: File?,
    private val ttlMillis: Long = TTL_MILLIS,
    private val staleLimitMillis: Long = STALE_LIMIT_MILLIS,
    private val now: () -> Long = System::currentTimeMillis,
    private val io: CoroutineContext = Dispatchers.IO,
) {
    private class Entry(val result: FeedResult, val at: Long)

    private val lock = Mutex()
    private val memory = mutableMapOf<String, Entry>()

    suspend fun load(query: FeedQuery, force: Boolean, client: FeedClient): FeedResult {
        val cached = lock.withLock { read(query) }
        if (!force && cached != null && now() - cached.at in 0 until ttlMillis) return cached.result
        val result = try {
            client.feed(query)
        } catch (e: CancellationException) {
            throw e
        } catch (e: Exception) {
            if (cached != null && now() - cached.at < staleLimitMillis) return cached.result.copy(stale = true)
            throw e
        }
        lock.withLock { write(query, Entry(result, now())) }
        return result
    }

    /** 覚えている行(古くても返す・取りに行かない)。取りに行く前に先に出す。無ければ null。 */
    suspend fun cached(query: FeedQuery): FeedResult? = lock.withLock { read(query) }?.result

    /** サインアウト・アカウント削除。 */
    suspend fun removeAll() {
        lock.withLock {
            memory.clear()
            val dir = directory ?: return
            withContext(io) { dir.deleteRecursively() }
        }
    }

    private suspend fun read(query: FeedQuery): Entry? {
        memory[query.cacheKey]?.let { return it }
        val file = file(query) ?: return null
        val text = withContext(io) { runCatching { if (file.exists()) file.readText() else null }.getOrNull() } ?: return null
        val entry = decode(text)
        if (entry == null) {
            withContext(io) { file.delete() }
            return null
        }
        memory[query.cacheKey] = entry
        return entry
    }

    private suspend fun write(query: FeedQuery, entry: Entry) {
        memory[query.cacheKey] = entry
        val file = file(query) ?: return
        val text = encode(entry)
        withContext(io) {
            runCatching { AtomicFile.write(file, text) }.onFailure { println("[Metamon] FeedCache store failed: $it") }
        }
    }

    /** ファイル名は鍵のハッシュ(diet の検索語をファイル名に出さない)。 */
    private fun file(query: FeedQuery): File? = directory?.resolve("${sha256(query.cacheKey)}.json")

    companion object {
        const val TTL_MILLIS: Long = 10 * 60 * 1000L
        const val STALE_LIMIT_MILLIS: Long = 24 * 60 * 60 * 1000L

        private val json = Json { ignoreUnknownKeys = true }

        private fun encode(e: Entry): String = buildJsonObject {
            put("at", e.at)
            put("result", json.parseToJsonElement(FeedResult.encode(e.result)))
        }.toString()

        private fun decode(text: String): Entry? = runCatching {
            val o = json.parseToJsonElement(text) as JsonObject
            val at = (o["at"] as JsonPrimitive).longOrNull ?: return null
            Entry(FeedResult.decode(o["result"].toString()), at)
        }.getOrNull()

        private fun sha256(s: String): String =
            MessageDigest.getInstance("SHA-256").digest(s.toByteArray()).joinToString("") { "%02x".format(it) }
    }
}
