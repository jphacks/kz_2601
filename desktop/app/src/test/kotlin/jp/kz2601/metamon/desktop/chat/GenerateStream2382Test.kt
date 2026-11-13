package jp.kz2601.metamon.desktop.chat

import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.services.GenerateProgress
import jp.kz2601.metamon.services.StreamedComponent
import jp.kz2601.metamon.services.http.FailureKind
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** 生成のストリームの受け口(#2382): testdata の SSE(正常・途中で壊れる・切れる)を 1 行ずつ流す。 */
class GenerateStream2382Test {
    private fun lines(name: String): List<String> =
        javaClass.getResourceAsStream("/generate-stream/$name.sse")!!.bufferedReader(Charsets.UTF_8).readLines()

    /** 1 行ずつ流し、描ける値が変わるたびに残す。 */
    private fun run(name: String): Pair<List<AppDSL>, GenerateEnd> {
        val stream = GenerateStream("app1")
        val seen = mutableListOf<AppDSL>()
        for (line in lines(name)) {
            stream.feed(line)
            stream.preview.value?.let { if (seen.lastOrNull() != it) seen += it }
        }
        return seen to stream.finish()
    }

    private fun AppDSL.componentTypes() = screens.first { it.id == initialScreen }.components.map { it.type }

    @Test
    fun normalStreamGrowsThenCompletes() {
        val (seen, end) = run("ok")
        assertEquals(3, seen.size)
        assertEquals("家計簿", seen[0].app.name)
        assertEquals(listOf("支出", "まとめ"), seen[0].app.tabs.map { it.title })
        assertEquals(listOf(ComponentType.TEXT), seen[1].componentTypes())
        assertEquals(listOf(ComponentType.TEXT, ComponentType.BUTTON), seen[2].componentTypes())
        assertEquals("expenses", seen[2].initialScreen)
        assertEquals("今月の支出", seen[2].screens.first().components[0].text)

        end as GenerateEnd.Completed
        assertEquals(2, end.result.document.version)
        assertEquals("家計簿を作りました", end.result.summary)
    }

    @Test
    fun brokenFramesKeepThePreviousValueThenFail() {
        val (seen, end) = run("broken")
        // 名前 → 部品 1 つ。壊れた行・知らない部品の値は流さない
        assertEquals(2, seen.size)
        assertEquals(listOf(ComponentType.TEXT), seen.last().componentTypes())
        end as GenerateEnd.Failed
        assertTrue("${end.kind}", end.kind is FailureKind.Busy)
    }

    @Test
    fun cutStreamEndsWithLastDrawableValue() {
        val (seen, end) = run("cut")
        // restart の後は組み直した値(前の部品は捨てる)
        assertEquals(listOf(ComponentType.BUTTON), seen.last().componentTypes())
        end as GenerateEnd.Cut
        assertEquals(seen.last(), end.last)
    }

    @Test
    fun restartKeepsShownValueUntilNextReadable() {
        val s = GenerateStream("app1")
        s.apply(GenerateProgress.App("家計簿", emptyList(), 0))
        val shown = s.preview.value
        s.apply(GenerateProgress.Restart(1))
        assertEquals(shown, s.preview.value)
    }

    @Test
    fun unreadableResultIsFailure() {
        val s = GenerateStream("app1")
        val end = s.readAll(sequenceOf("event: result", """data: {"appId":"app1","version":2}""", ""))
        end as GenerateEnd.Failed
        assertTrue("${end.kind}", end.kind is FailureKind.Other)
    }

    @Test
    fun linesAfterEndAreIgnoredAndEmptyStreamIsCut() {
        val empty = GenerateStream("app1")
        assertEquals(GenerateEnd.Cut(null), empty.readAll(emptySequence()))

        val s = GenerateStream("app1")
        s.readAll(sequenceOf("event: error", """data: {"error":{"code":"x","message":"m","status":500}}""", ""))
        s.feed("event: app")
        s.feed("""data: {"name":"後から","tabs":[]}""")
        s.feed("")
        assertNull(s.preview.value)
        assertTrue(s.end is GenerateEnd.Failed)
    }

    @Test
    fun previewNeedsSomethingToDraw() {
        assertNull(GenerateStream.previewDsl(null, emptyList(), emptyMap()))
        val unknown = StreamedComponent(attempt = 0, index = 0, type = "hologram")
        assertNull(GenerateStream.previewDsl("x", emptyList(), mapOf("" to listOf(unknown))))
        val ok = GenerateStream.previewDsl(null, emptyList(), mapOf("" to listOf(unknown.copy(type = "text", label = "a"))))!!
        assertEquals(GenerateStream.FIRST_SCREEN, ok.initialScreen)
    }
}
