package jp.kz2601.metamon.services.http

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class SseParserTest {
    @Test
    fun framesSplitAcrossChunks() {
        val p = SseParser()
        assertTrue(p.feed("event: app\nda").isEmpty())
        assertTrue(p.feed("ta: {\"name\":\"x\"}\n").isEmpty())
        assertEquals(listOf(SseFrame("app", """{"name":"x"}""")), p.feed("\n"))
    }

    @Test
    fun crlfCommentsAndUnknownFieldsAreHandled() {
        val frames = SseParser().feed(": keep-alive\r\nid: 3\r\nretry: 1000\r\nevent: restart\r\ndata: {\"attempt\":2}\r\n\r\n")
        assertEquals(listOf(SseFrame("restart", """{"attempt":2}""")), frames)
    }

    @Test
    fun multipleDataLinesJoinWithNewline() {
        val frames = SseParser().feed("data: a\ndata: b\n\n")
        assertEquals(listOf(SseFrame("message", "a\nb")), frames)
    }

    @Test
    fun blankLineWithoutDataEmitsNothingAndResetsEvent() {
        val p = SseParser()
        assertTrue(p.feed("event: app\n\n").isEmpty())
        assertEquals(listOf(SseFrame("message", "x")), p.feed("data: x\n\n"))
    }

    @Test
    fun truncatedLastFrameIsNotEmitted() {
        val p = SseParser()
        assertTrue(p.feed("event: result\ndata: {}\n").isEmpty())
        assertNull(p.feedLine("data: more"))
    }

    @Test
    fun valueWithoutSpaceAfterColon() {
        assertEquals(listOf(SseFrame("app", "x")), SseParser().feed("event:app\ndata:x\n\n"))
    }
}
