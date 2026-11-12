package jp.kz2601.metamon.renderer.components.ai

import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.DslJson
import jp.kz2601.metamon.dsl.Screen
import jp.kz2601.metamon.dsl.ai.AiTextRequest
import jp.kz2601.metamon.dsl.ai.AiTextShape
import jp.kz2601.metamon.dsl.feed.FeedItem
import jp.kz2601.metamon.dsl.feed.FeedQuery
import jp.kz2601.metamon.dsl.feed.FeedResult
import jp.kz2601.metamon.dsl.feed.FeedSource
import jp.kz2601.metamon.renderer.components.feed.FeedHost
import kotlinx.coroutines.delay
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.IOException

/** aiButton `inputFeeds`: 同じ画面の feed の行を「外部データ」として材料に足す(iOS の AIFeedInputTests・20 行・2 KB・節を優先して残す)。 */
class AiFeedInputTest {
    private val source = FeedSource(name = "気象庁ホームページ", url = "https://www.jma.go.jp/", license = "公共データ利用規約(第1.0版)")

    private fun screen(json: String): Screen = DslJson.decodeFromString(Screen.serializer(), json)

    private fun component(json: String): Component = DslJson.decodeFromString(Component.serializer(), json)

    private fun Screen.find(id: String): Component = (components + components.flatMap { it.children }).first { it.id == id }

    @Test
    fun feedsOnSameScreenInOrder() {
        val s = screen(
            """
            {"id":"home","title":"体調","toolbar":[],"components":[
             {"type":"feed","id":"wx","provider":"weather","region":"170000"},
             {"type":"vstack","children":[{"type":"feed","id":"hpa","provider":"pressure","region":"170000"},
              {"type":"text","id":"note","text":"メモ"}]},
             {"type":"aiButton","id":"b","task":"generate","prompt":"体調","inputFeeds":["hpa","wx","note","nope"]}]}
            """.trimIndent(),
        )
        val button = s.find("b")
        assertEquals(listOf("hpa", "wx", "note", "nope"), button.inputFeeds)
        assertEquals("feed だけ・inputFeeds の順", listOf("hpa", "wx"), AiFeedInput.feeds(button, s).map { it.id })
        assertTrue(AiFeedInput.feeds(button, null).isEmpty())
    }

    @Test
    fun lines() {
        val wx = FeedItem(date = "2026-11-07", title = "晴れ", subtitle = "晴れ 時々 くもり", max = 13.4, min = 8.6, pop = 10)
        assertEquals("- 11/7(土) / 晴れ / 晴れ 時々 くもり / 最高 13° / 最低 9° / 降水確率 10%", AiFeedInput.line(wx, "weather", english = false))
        val hpa = FeedItem(date = "2026-11-09T06:00:00+09:00", title = "6時〜12時", max = 1013.4, min = 1006.2, drop = true)
        assertEquals("- 11/9(月) / 6時〜12時 / 1006〜1013 hPa / 気圧が大きく下がる", AiFeedInput.line(hpa, "pressure", english = false))
        assertEquals("- お知らせ", AiFeedInput.line(FeedItem(title = "お知らせ"), "rss", english = true))
    }

    @Test
    fun sectionSharesRowsAndBytes() {
        val many = (0 until 30).map { FeedItem(date = "2026-11-07", title = "行$it") }
        val a = FeedResult(provider = "weather", title = "石川県の天気", items = many, source = source)
        val b = FeedResult(provider = "holidays", title = "国民の祝日", items = many, source = source)
        val s = AiFeedInput.section(listOf(AiFeedInput.Loaded("天気", a), AiFeedInput.Loaded("", b)), english = false)
        assertTrue(s.startsWith("外部データ(アプリが表示している公開データ):\n■ 天気\n"))
        assertTrue("部品の title が無ければ結果の title", s.contains("\n■ 国民の祝日\n"))
        assertEquals("合わせて 20 行(1 つ 10 行)", 20, s.split("\n").count { it.startsWith("- ") })

        val longRow = "長".repeat(100)
        val long = (0 until 20).map { FeedItem(title = longRow) }
        val big = AiFeedInput.section(listOf(AiFeedInput.Loaded("", FeedResult(provider = "rss", title = "お知らせ", items = long, source = source))), english = false)
        assertTrue(big.toByteArray(Charsets.UTF_8).size <= AiFeedInput.MAX_BYTES)
        assertTrue("行の途中で切らない", big.endsWith(longRow))

        val empty = AiFeedInput.section(listOf(AiFeedInput.Loaded("", FeedResult(provider = "warnings", title = "警報", items = emptyList(), source = source))), english = false)
        assertTrue("取れて 0 行は「今は無い」(警報なし等)", empty.endsWith("- (今は表示するものがありません)"))
    }

    /** 取れなかった feed は 1 行で知らせて続ける。1 つも取れなければ「外部データ: なし」(AI がデータを作らないように)。 */
    @Test
    fun sectionFailedAndNone() {
        val a = FeedResult(provider = "weather", title = "石川県の天気", items = listOf(FeedItem(title = "晴れ")), source = source)
        val mixed = AiFeedInput.section(listOf(AiFeedInput.Loaded("天気", a), AiFeedInput.Loaded("気圧", null)), english = false)
        assertTrue(mixed, mixed.endsWith("■ 気圧\n- (取得できませんでした)"))
        assertTrue(mixed.contains("- 晴れ"))
        assertTrue(AiFeedInput.section(listOf(AiFeedInput.Loaded("天気", a), AiFeedInput.Loaded("気圧", null)), english = true).endsWith("- (could not be loaded)"))

        val none = "外部データ: なし(添えられるデータがありません。外部のデータを推測・作成しないでください)"
        assertEquals(none, AiFeedInput.section(emptyList(), english = false))
        assertEquals(none, AiFeedInput.section(listOf(AiFeedInput.Loaded("気圧", null)), english = false))
        assertTrue(AiFeedInput.section(emptyList(), english = true).startsWith("External data: none"))
    }

    @Test
    fun mergeKeepsSection() {
        assertEquals("外部", AiFeedInput.merge(null, "外部", 4096))
        assertEquals("記録", AiFeedInput.merge("記録", null, 4096))
        assertEquals("記録\n\n外部", AiFeedInput.merge("記録", "外部", 4096))
        val merged = AiFeedInput.merge("a".repeat(5000), "b".repeat(2000), 4096).orEmpty()
        assertEquals(4096, merged.toByteArray(Charsets.UTF_8).size)
        assertTrue("はみ出す分は元の材料を削る", merged.endsWith("b".repeat(2000)))
    }

    // MARK: 読み込み

    private inner class FakeHost : FeedHost {
        var cached: Map<String, FeedResult> = emptyMap()
        var delays: Map<String, Long> = emptyMap()
        var failing: Set<String> = emptySet()
        val calls = mutableListOf<String>()

        /** 同時に待っている数と、その最大(順に待つと 1)。 */
        private var inFlight = 0
        var maxInFlight = 0
            private set

        override suspend fun feed(query: FeedQuery, force: Boolean): FeedResult {
            synchronized(this) {
                calls += query.provider
                inFlight += 1
                maxInFlight = maxOf(maxInFlight, inFlight)
            }
            try {
                delay(delays[query.provider] ?: 0)
                if (query.provider in failing) throw IOException("offline")
                return FeedResult(provider = query.provider, title = "取得 ${query.provider}", items = listOf(FeedItem(title = "新しい行")), source = source)
            } finally {
                synchronized(this) { inFlight -= 1 }
            }
        }

        override suspend fun cachedFeed(query: FeedQuery): FeedResult? = cached[query.provider]
    }

    private fun feedScreen(): Pair<Screen, Component> {
        val s = screen(
            """
            {"id":"home","title":"体調","toolbar":[],"components":[
             {"type":"feed","id":"wx","title":"天気","provider":"weather","region":"170000"},
             {"type":"feed","id":"hpa","provider":"pressure","region":"170000"},
             {"type":"feed","id":"hol","provider":"holidays"},
             {"type":"aiButton","id":"b","task":"generate","prompt":"体調","inputFeeds":["wx","hpa","hol"]}]}
            """.trimIndent(),
        )
        return s to s.find("b")
    }

    /** 同時に取りに行く(1 つずつ待たない)。取れない feed は null で、残りは使う。 */
    @Test
    fun loadConcurrentlyAndKeepsGoingOnFailure() = runBlocking<Unit> {
        val (s, button) = feedScreen()
        val host = FakeHost().apply {
            delays = mapOf("weather" to 400L, "pressure" to 400L, "holidays" to 400L)
            failing = setOf("pressure")
        }
        val loaded = AiFeedInput.load(AiFeedInput.feeds(button, s), host, seconds = 5.0)
        assertEquals("3 つ × 0.4 秒を順に待たない", 3, host.maxInFlight)
        assertEquals(listOf("天気", "pressure", ""), loaded.map { it.title })
        assertEquals(listOf("取得 weather", null, "取得 holidays"), loaded.map { it.result?.title })
    }

    /** 全体で seconds まで。間に合わない feed は「取得できませんでした」になり、取れた分で進む。 */
    @Test
    fun loadHasOverallBudget() = runBlocking<Unit> {
        val (s, button) = feedScreen()
        val host = FakeHost().apply { delays = mapOf("weather" to 0L, "pressure" to 30_000L, "holidays" to 30_000L) }
        val started = System.nanoTime()
        val request = AiFeedInput.attach(button, s, host, AiTextRequest(instruction = "体調", shape = AiTextShape.LINE, locale = "ja"), seconds = 0.2)
        assertTrue("遅い feed を待ち続けない", (System.nanoTime() - started) / 1_000_000 < 2000)
        val input = request.input!!
        assertTrue(input, input.contains("■ 天気\n- 新しい行"))
        assertTrue(input, input.contains("■ pressure\n- (取得できませんでした)"))
    }

    /** 覚えている行は古くてもそのまま使う(画面が出している行と同じ・取り直さない)。 */
    @Test
    fun loadPrefersCachedRows() = runBlocking<Unit> {
        val (s, button) = feedScreen()
        val host = FakeHost().apply {
            cached = mapOf("weather" to FeedResult(provider = "weather", title = "前の天気", items = listOf(FeedItem(title = "画面の行")), source = source))
        }
        val loaded = AiFeedInput.load(AiFeedInput.feeds(button, s), host, seconds = 5.0)
        assertEquals("画面の行", loaded.first().result?.items?.first()?.title)
        assertEquals("覚えている feed は取りに行かない", listOf("holidays", "pressure"), host.calls.sorted())
    }

    /** inputFeeds があるのに 1 行も添えられない(feed が消えた・ホストが無い)時は「外部データ: なし」を書く。 */
    @Test
    fun attachStatesNoneWhenNothingAttached() = runBlocking<Unit> {
        val (s, button) = feedScreen()
        val gone = component("""{"type":"aiButton","id":"b2","task":"generate","prompt":"体調","inputFeeds":["removed"]}""")
        val withInput = AiTextRequest(instruction = "体調", shape = AiTextShape.LINE, input = "記録", locale = "ja")
        assertEquals(
            "記録\n\n外部データ: なし(添えられるデータがありません。外部のデータを推測・作成しないでください)",
            AiFeedInput.attach(gone, s, FakeHost(), withInput).input,
        )

        val noHost = AiFeedInput.attach(button, s, null, AiTextRequest(instruction = "体調", shape = AiTextShape.LINE, locale = "en"))
        assertTrue(noHost.input.orEmpty().startsWith("External data: none"))

        val noFeeds = component("""{"type":"aiButton","id":"b3","task":"generate","prompt":"体調"}""")
        val plain = AiFeedInput.attach(noFeeds, s, null, AiTextRequest(instruction = "体調", shape = AiTextShape.LINE, locale = "ja"))
        assertNull("inputFeeds が無ければ何も足さない", plain.input)
    }
}
