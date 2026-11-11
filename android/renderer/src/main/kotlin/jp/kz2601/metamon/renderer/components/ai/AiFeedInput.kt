package jp.kz2601.metamon.renderer.components.ai

import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.dsl.Screen
import jp.kz2601.metamon.dsl.ai.AiTextLimits
import jp.kz2601.metamon.dsl.ai.AiTextRequest
import jp.kz2601.metamon.dsl.expr.SwiftNumbers
import jp.kz2601.metamon.dsl.feed.FeedItem
import jp.kz2601.metamon.dsl.feed.FeedQuery
import jp.kz2601.metamon.dsl.feed.FeedResult
import jp.kz2601.metamon.renderer.components.feed.FeedFormat
import jp.kz2601.metamon.renderer.components.feed.FeedHost
import kotlinx.coroutines.CancellationException
import kotlinx.coroutines.coroutineScope
import kotlinx.coroutines.joinAll
import kotlinx.coroutines.launch
import kotlinx.coroutines.withTimeoutOrNull

/**
 * aiButton `inputFeeds`(iOS の `AIFeedInput`): 同じ画面の `feed` が出している行を、generate の材料の末尾に「外部データ」の節として足す。
 * 行はホストの [FeedHost](同じ問い合わせはキャッシュ = 画面の表示と同じ行)から取る。合わせて 20 行・2 KB まで。
 * 検証は backend `dsl/ai_input_feed.go`。
 */
object AiFeedInput {
    const val MAX_ROWS = 20
    const val MAX_BYTES = 2048

    /** 覚えていない feed を取りに行く時に待つ長さ(秒・全部の feed を合わせて)。 */
    const val WAIT_SECONDS = 8.0

    /** feed 1 つ分の行。[result] が null なら取れなかった。 */
    data class Loaded(val title: String, val result: FeedResult?)

    /** 材料と節を合わせる。節を残し、はみ出す分は元の材料(記録・フォーム)を先に削る。 */
    fun merge(input: String?, section: String?, maxBytes: Int): String? {
        if (section.isNullOrEmpty()) return input
        val sep = "\n\n"
        if (input.isNullOrEmpty()) return AiTextLimits.clip(section, maxBytes)
        val room = maxBytes - section.utf8Size() - sep.utf8Size()
        if (room <= 0) return AiTextLimits.clip(section, maxBytes)
        return AiTextLimits.clip(input, room) + sep + section
    }

    /** 画面の部品(器の中も 1 段・iOS の `Screen.allComponents`)。 */
    private fun Screen.allComponents(): List<Component> = components.flatMap { listOf(it) + it.children }

    /** `inputFeeds` の順に、同じ画面の feed 部品。知らない id・feed でない部品は飛ばす。 */
    fun feeds(c: Component, screen: Screen?): List<Component> {
        if (c.inputFeeds.isEmpty() || screen == null) return emptyList()
        val all = screen.allComponents()
        return c.inputFeeds.mapNotNull { id -> all.firstOrNull { it.id == id && it.type == ComponentType.FEED } }
    }

    /** 1 行: 日付 / 見出し / 補足 / 値(最高・最低・降水確率・気圧の幅・急な低下)。 */
    fun line(item: FeedItem, provider: String, english: Boolean): String {
        val parts = mutableListOf<String>()
        (FeedFormat.dateText(item.date, provider) ?: item.date)?.takeIf { it.isNotEmpty() }?.let { parts += it }
        parts += item.title
        item.subtitle?.takeIf { it.isNotEmpty() && it != item.title }?.let { parts += it }
        fun f(v: Double) = SwiftNumbers.rounded(v).toLong().toString()
        if (provider == "pressure") {
            FeedFormat.pressureRange(item)?.let { parts += it }
            if (item.drop == true) parts += if (english) "large pressure drop" else "気圧が大きく下がる"
        } else {
            item.max?.let { parts += if (english) "high ${f(it)}°C" else "最高 ${f(it)}°" }
            item.min?.let { parts += if (english) "low ${f(it)}°C" else "最低 ${f(it)}°" }
        }
        item.pop?.let { parts += if (english) "rain $it%" else "降水確率 $it%" }
        return "- " + parts.joinToString(" / ")
    }

    /**
     * 「外部データ」の節。行は feed の数で分け合い(合わせて [MAX_ROWS])、全体を [MAX_BYTES] に行の単位で切る。
     * 取れなかった feed は「(取得できませんでした)」の 1 行。1 つも取れない(feed が無い・全部取れない)時は
     * 「外部データ: なし」とだけ書く(AI が外部のデータを作らないように)。
     */
    fun section(feeds: List<Loaded>, english: Boolean): String {
        if (feeds.none { it.result != null }) {
            return if (english) {
                "External data: none (nothing could be attached. Do not assume or invent external data.)"
            } else {
                "外部データ: なし(添えられるデータがありません。外部のデータを推測・作成しないでください)"
            }
        }
        val perFeed = maxOf(MAX_ROWS / feeds.size, 1)
        val lines = mutableListOf(if (english) "External data (public data shown in the app):" else "外部データ(アプリが表示している公開データ):")
        for (feed in feeds) {
            lines += "■ " + feed.title.ifEmpty { feed.result?.title.orEmpty() }
            val result = feed.result
            if (result == null) {
                lines += if (english) "- (could not be loaded)" else "- (取得できませんでした)"
                continue
            }
            val rows = result.items.take(perFeed).map { line(it, result.provider, english) }
            lines += rows.ifEmpty { listOf(if (english) "- (nothing to show now)" else "- (今は表示するものがありません)") }
        }
        var out = ""
        for (l in lines) {
            val next = if (out.isEmpty()) l else out + "\n" + l
            if (next.utf8Size() > MAX_BYTES) break
            out = next
        }
        return out
    }

    /** `c.inputFeeds` があれば、feed の行を材料に足す。取れない feed があっても止めない(節にそう書く)。 */
    suspend fun attach(c: Component, screen: Screen?, host: FeedHost?, request: AiTextRequest, seconds: Double = WAIT_SECONDS): AiTextRequest {
        if (c.inputFeeds.isEmpty()) return request
        val loaded = load(feeds(c, screen), host, seconds)
        val maxInput = AiTextLimits.maxInputBytes(request.isLong)
        return request.copy(input = merge(request.input, section(loaded, english = request.locale == "en"), maxInput))
    }

    /**
     * feed の行を集める。覚えている行(古くても = 画面が出している行)を先に使い、無い feed だけを同時に取りに行く
     * (合わせて [seconds] まで)。取れない・間に合わない feed は result が null。順は [comps] のまま。
     */
    suspend fun load(comps: List<Component>, host: FeedHost?, seconds: Double): List<Loaded> {
        val queries = comps.map { FeedQuery.of(it) }
        val results = arrayOfNulls<FeedResult>(comps.size)
        queries.forEachIndexed { i, q -> if (q != null && host != null) results[i] = cached(host, q) }
        val missing = results.indices.filter { results[it] == null && queries[it] != null }
        if (host != null && missing.isNotEmpty()) {
            coroutineScope {
                val jobs = missing.map { i ->
                    launch {
                        val got = try {
                            host.feed(queries[i]!!, force = false)
                        } catch (e: CancellationException) {
                            throw e
                        } catch (_: Exception) {
                            null
                        }
                        synchronized(results) { results[i] = got }
                    }
                }
                withTimeoutOrNull((seconds * 1000).toLong()) { jobs.joinAll() } // 待ちきったら残りは取れなかった扱い
                jobs.forEach { it.cancel() }
            }
        }
        return comps.indices.map { i ->
            val title = comps[i].title.orEmpty()
            val r = synchronized(results) { results[i] }
            Loaded(if (title.isEmpty() && r == null) comps[i].provider?.wire.orEmpty() else title, r)
        }
    }

    private suspend fun cached(host: FeedHost, q: FeedQuery): FeedResult? = try {
        host.cachedFeed(q)
    } catch (e: CancellationException) {
        throw e
    } catch (_: Exception) {
        null
    }

    private fun String.utf8Size(): Int = toByteArray(Charsets.UTF_8).size
}
