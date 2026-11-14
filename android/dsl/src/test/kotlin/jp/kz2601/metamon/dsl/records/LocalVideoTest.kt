package jp.kz2601.metamon.dsl.records

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.nio.file.Files

class LocalVideoTest {
    private val thumb = "data:image/jpeg;base64,AAAA"

    @Test
    fun encodesLikeIos() {
        assertEquals("video:abc-1|$thumb", VideoValue("abc-1", thumb).encoded)
        assertEquals("video:abc-1", VideoValue("abc-1").encoded)
        assertEquals("https://example.com/a.mp4", VideoValue(null, remoteUrl = "https://example.com/a.mp4").encoded)
    }

    @Test
    fun parsesAndRoundTrips() {
        assertEquals(VideoValue("abc_1", thumb), VideoValue.parse("video:abc_1|$thumb"))
        assertEquals(VideoValue("abc_1"), VideoValue.parse("video:abc_1"))
        // サムネイルが data:image/ でなければ捨てる
        assertEquals(VideoValue("abc_1"), VideoValue.parse("video:abc_1|javascript:x"))
        assertEquals("https://e.com/v.mp4", VideoValue.parse("https://e.com/v.mp4")?.remoteUrl)
        val v = VideoValue("id9", thumb)
        assertEquals(v, VideoValue.parse(v.encoded))
        assertEquals(v, VideoValue.of(JSONValue.Str(v.encoded)))
    }

    @Test
    fun rejectsOtherShapes() {
        assertNull(VideoValue.parse("video:"))
        assertNull(VideoValue.parse("video:../etc|$thumb"))
        assertNull(VideoValue.parse("video:a/b"))
        assertNull(VideoValue.parse("http://e.com/v.mp4"))
        assertNull(VideoValue.parse(thumb))
        assertNull(VideoValue.of(JSONValue.Null))
        assertNull(VideoValue.of(null))
    }

    @Test
    fun idRules() {
        assertTrue(LocalVideoStore.isValidId("3F2504E0-4F89-11D3-9A0C-0305E82C3301"))
        assertFalse(LocalVideoStore.isValidId(""))
        assertFalse(LocalVideoStore.isValidId("a".repeat(81)))
        assertFalse(LocalVideoStore.isValidId(".."))
        assertFalse(LocalVideoStore.isValidId("動画"))
    }

    @Test
    fun localIdsInRecord() {
        val data = mapOf(
            "a" to JSONValue.Str("video:one|$thumb"),
            "b" to JSONValue.Str("https://e.com/v.mp4"),
            "c" to JSONValue.Str("text"),
            "d" to JSONValue.Str("video:two"),
        )
        assertEquals(setOf("one", "two"), VideoValue.localIds(data))
    }

    @Test
    fun storeImportsAndRemoves() {
        val dir = Files.createTempDirectory("videos").toFile()
        try {
            val store = LocalVideoStore(dir.resolve("metamon-videos"))
            val src = dir.resolve("tmp.mp4").apply { writeText("x") }
            val id = store.importFile(src)
            assertTrue(store.exists(id))
            assertFalse(src.exists())
            assertEquals("$id.mp4", store.file(id)!!.name)
            assertNull(store.file("../x"))
            store.remove(listOf(id))
            assertFalse(store.exists(id))
            store.importFile(dir.resolve("t2.mp4").apply { writeText("y") }, "fixed")
            store.removeAll()
            assertFalse(store.directory.exists())
        } finally {
            dir.deleteRecursively()
        }
    }
}
