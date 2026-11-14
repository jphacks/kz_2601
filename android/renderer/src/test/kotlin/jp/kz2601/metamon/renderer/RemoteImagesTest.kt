package jp.kz2601.metamon.renderer

import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.Field
import jp.kz2601.metamon.dsl.records.FieldType
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.renderer.components.ImageGuard
import jp.kz2601.metamon.renderer.components.PhotoValue
import jp.kz2601.metamon.renderer.components.RemoteImages
import jp.kz2601.metamon.renderer.components.photoFieldValue
import jp.kz2601.metamon.renderer.form.FormDraft
import jp.kz2601.metamon.renderer.navigation.Route
import jp.kz2601.metamon.renderer.registry.RenderContext
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.mockwebserver.MockResponse
import okhttp3.mockwebserver.MockWebServer
import okio.buffer
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder
import java.io.IOException

/** https の画像: 開いてよい URL・field から読む値・大きさの上限・後片付け。 */
class RemoteImagesTest {
    @get:Rule
    val tmp = TemporaryFolder()

    @Test
    fun onlyHttpsWithHostIsLoadable() {
        assertEquals("https://example.com/a.jpg", RemoteImages.loadableUrl("https://example.com/a.jpg"))
        assertEquals("https://example.com/a.jpg", RemoteImages.loadableUrl("  https://example.com/a.jpg "))
        assertEquals("HTTPS://example.com/a.jpg", RemoteImages.loadableUrl("HTTPS://example.com/a.jpg"))
        assertNull(RemoteImages.loadableUrl("http://example.com/a.jpg"))
        assertNull(RemoteImages.loadableUrl("https:///a.jpg"))
        assertNull(RemoteImages.loadableUrl("https://user:pass@example.com/a.jpg"))
        assertNull(RemoteImages.loadableUrl("file:///sdcard/a.jpg"))
        assertNull(RemoteImages.loadableUrl("javascript:alert(1)"))
        assertNull(RemoteImages.loadableUrl("data:image/jpeg;base64,/9j/"))
        assertNull(RemoteImages.loadableUrl("https://example.com/a b.jpg"))
        assertNull(RemoteImages.loadableUrl("https://example.com/" + "a".repeat(RemoteImages.MAX_URL_LENGTH)))
        assertNull(RemoteImages.loadableUrl(""))
        assertNull(RemoteImages.loadableUrl(null))
        // http は写真なし扱い(枠だけ)
        assertFalse(PhotoValue.hasPhoto("http://example.com/a.jpg"))
        assertTrue(PhotoValue.hasPhoto("https://example.com/a.jpg"))
    }

    @Test
    fun recordPhotoValue() {
        // 一覧の行の写真(item.image)は記録の値をそのまま読む。動画はサムネイル
        val jpeg = "data:image/jpeg;base64,/9j/4AAQ"
        assertEquals(jpeg, PhotoValue.of(JSONValue.Str(jpeg)))
        assertEquals("https://example.com/a.jpg", PhotoValue.of(JSONValue.Str("https://example.com/a.jpg")))
        assertEquals(jpeg, PhotoValue.of(JSONValue.Str("video:clip-1|$jpeg")))
        assertNull(PhotoValue.of(JSONValue.Str("video:clip-1")))
        assertNull(PhotoValue.of(JSONValue.Str("http://example.com/a.jpg")))
        assertNull(PhotoValue.of(JSONValue.Str("")))
        assertNull(PhotoValue.of(JSONValue.Num(1.0)))
        assertNull(PhotoValue.of(JSONValue.Null))
        assertNull(PhotoValue.of(null))
    }

    private val entity = Entity(id = "profile", fields = listOf(Field("photo", type = FieldType.PHOTO), Field("name")))

    private fun ctx(vararg values: Pair<String, JSONValue>): RenderContext {
        val form = FormDraft(entity).also { f -> values.forEach { (k, v) -> f.values[k] = v } }
        return RenderContext(Route("home"), form = form)
    }

    @Test
    fun imageAndAvatarReadPhotoFromFormDraft() {
        val image = Component(type = ComponentType.IMAGE, field = "photo")
        val https = "https://example.com/me.jpg"
        val data = "data:image/jpeg;base64,/9j/4AAQ"
        assertEquals(https, photoFieldValue(image, ctx("photo" to JSONValue.Str(https))))
        assertEquals(data, photoFieldValue(image, ctx("photo" to JSONValue.Str(data))))
        // 空・写真でない値・フォームの外は無し
        assertNull(photoFieldValue(image, ctx("photo" to JSONValue.Null)))
        assertNull(photoFieldValue(image, ctx("photo" to JSONValue.Str(""))))
        assertNull(photoFieldValue(image, ctx("photo" to JSONValue.Str("http://example.com/me.jpg"))))
        assertNull(photoFieldValue(image, ctx("name" to JSONValue.Str(https))))
        assertNull(photoFieldValue(image, null))
        assertNull(photoFieldValue(Component(type = ComponentType.IMAGE), ctx("photo" to JSONValue.Str(https))))
        // video field の値はサムネイル
        val thumb = "data:image/png;base64,iVBORw0K"
        val video = "video:" + "0123456789abcdef0123456789abcdef" + "|" + thumb
        assertEquals(thumb, photoFieldValue(image, ctx("photo" to JSONValue.Str(video))))
        assertNull(photoFieldValue(image, ctx("photo" to JSONValue.Str("video:0123456789abcdef"))))
    }

    @Test
    fun guardStopsLargeAndNonHttpsResponses() {
        MockWebServer().use { server ->
            server.enqueue(MockResponse().setBody("x".repeat(2_000)))
            server.enqueue(MockResponse().setChunkedBody("x".repeat(2_000), 100))
            server.enqueue(MockResponse().setBody("x".repeat(500)))
            server.enqueue(MockResponse().setBody("ok"))
            server.start()
            val small = OkHttpClient.Builder().addNetworkInterceptor(ImageGuard(maxBytes = 1_000, requireHttps = false)).build()
            fun get(client: OkHttpClient) = client.newCall(Request.Builder().url(server.url("/a.jpg")).build()).execute().use { it.body.string() }

            assertThrows { get(small) } // Content-Length が上限超え
            assertThrows { get(small) } // chunked で読みながら上限超え
            assertEquals(500, get(small).length)
            // https 必須の時は http を読まない
            val strict = OkHttpClient.Builder().addNetworkInterceptor(ImageGuard()).build()
            assertThrows { get(strict) }
        }
    }

    /** gzip で小さく送って、解くと上限を超える応答も止める。 */
    @Test
    fun guardCountsDecompressedBytes() {
        MockWebServer().use { server ->
            val gz = okio.Buffer()
            okio.GzipSink(gz).buffer().use { it.writeUtf8("x".repeat(5_000)) }
            assertTrue(gz.size < 1_000)
            server.enqueue(MockResponse().setBody(gz).addHeader("Content-Encoding", "gzip"))
            server.enqueue(MockResponse().setBody("x".repeat(500)))
            server.start()
            val images = RemoteImages.imageClient(OkHttpClient(), maxBytes = 1_000, requireHttps = false)
            fun get() = images.newCall(Request.Builder().url(server.url("/a.jpg")).build()).execute().use { it.body.string() }
            assertThrows { get() }
            assertEquals(500, get().length)
        }
    }

    @Test
    fun wipeRemovesDiskCacheEvenBeforeLoaderStarts() {
        val dir = tmp.newFolder(RemoteImages.DISK_DIR)
        dir.resolve("journal").writeText("x")
        dir.resolve("abc.1").writeBytes(ByteArray(10))
        RemoteImages.wipe(dir)
        assertFalse(dir.exists())
    }

    private fun assertThrows(block: () -> Unit) {
        try {
            block()
            fail("IOException が出るはず")
        } catch (_: IOException) {
        }
    }
}
