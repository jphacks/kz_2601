package jp.kz2601.metamon.renderer

import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.dsl.decode
import jp.kz2601.metamon.dsl.logic.ExternalUrlPolicy
import jp.kz2601.metamon.renderer.components.music.MusicSearch
import jp.kz2601.metamon.renderer.components.music.MusicSong
import jp.kz2601.metamon.renderer.conformance.RenderTree
import jp.kz2601.metamon.renderer.navigation.rootRouteFor
import jp.kz2601.metamon.renderer.registry.ComponentRegistry
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** musicPlayer の代わり(#879): props の読み方・検索の URL・符号化(iOS の MusicLinks / MusicRequestRules と同じ)。 */
class MusicLeavesTest {
    private fun dsl(component: String) = AppDSL.decode(
        """
        {"schemaVersion":1,"app":{"name":"t","voiceEntry":"none"},"initialScreen":"home",
         "screens":[{"id":"home","title":"BGM","components":[$component]}]}
        """.trimIndent(),
    )

    @Test
    fun registered() {
        assertTrue(ComponentType.MUSIC_PLAYER.wire in ComponentRegistry.standard.registeredTypes)
    }

    @Test
    fun headingReadsTitleLikeIos() {
        val withTitle = dsl("""{"type":"musicPlayer","id":"p","title":"ドライブ BGM","allowRequests":true,"autoAccept":false}""")
        val c = withTitle.screens.first().components.first()
        assertEquals("ドライブ BGM", MusicSearch.heading(c, "音楽"))
        assertEquals(true, c.allowRequests)
        val noTitle = dsl("""{"type":"musicPlayer"}""").screens.first().components.first()
        assertEquals("音楽", MusicSearch.heading(noTitle, "音楽"))
        // iOS は `c.title ?? "音楽"`: 空文字はそのまま
        val empty = dsl("""{"type":"musicPlayer","title":""}""").screens.first().components.first()
        assertEquals("", MusicSearch.heading(empty, "音楽"))
    }

    @Test
    fun conformanceNodeIsRenderedWithDeclaredTitle() {
        val d = dsl("""{"type":"musicPlayer","id":"player","title":"ドライブ BGM"}""")
        val tree = RenderTree(d, ComponentRegistry.standard.registeredTypes)
        val node = tree.screen(d.rootRouteFor("home"))["nodes"]!!.jsonArray.first().jsonObject
        assertEquals("rendered", (node["state"] as JsonPrimitive).content)
        assertEquals(JsonObject(mapOf("title" to JsonPrimitive("ドライブ BGM"))), node["props"])
    }

    @Test
    fun sanitizeTrimsClipsAndNeedsTitle() {
        assertNull(MusicSearch.sanitize(MusicSong("  ", "someone")))
        assertEquals(MusicSong("夜に駆ける", "YOASOBI"), MusicSearch.sanitize(MusicSong(" 夜に駆ける ", "\tYOASOBI\n")))
        val long = MusicSearch.sanitize(MusicSong("a".repeat(150), "b".repeat(150)))!!
        assertEquals(MusicSearch.MAX_TITLE, long.title.length)
        assertEquals(MusicSearch.MAX_ARTIST, long.artist.length)
    }

    @Test
    fun searchTermJoinsNonEmptyParts() {
        assertEquals("夜に駆ける YOASOBI", MusicSearch.searchTerm(MusicSong("夜に駆ける", "YOASOBI")))
        assertEquals("夜に駆ける", MusicSearch.searchTerm(MusicSong("夜に駆ける", "  ")))
        assertNull(MusicSearch.searchUrl(MusicSong(" ", "")))
    }

    @Test
    fun searchUrlEncodesLikeIos() {
        assertEquals("https://open.spotify.com/search/Let%20It%20Be%20The%20Beatles", MusicSearch.searchUrl(MusicSong("Let It Be", "The Beatles")))
        assertEquals(
            "https://open.spotify.com/search/%E5%A4%9C%E3%81%AB%E9%A7%86%E3%81%91%E3%82%8B",
            MusicSearch.searchUrl(MusicSong("夜に駆ける")),
        )
        // パスを抜ける・クエリやフラグメントになる文字は符号化する
        assertEquals("https://open.spotify.com/search/AC%2FDC%3F%23x%25", MusicSearch.searchUrl(MusicSong("AC/DC?#x%")))
        // urlPathAllowed に入る記号はそのまま
        assertEquals("https://open.spotify.com/search/Rock&Roll!-_.~:@", MusicSearch.searchUrl(MusicSong("Rock&Roll!-_.~:@")))
    }

    @Test
    fun searchUrlIsAlwaysOpenable() {
        for (t in listOf("javascript:alert(1)", "intent://x#Intent;end", "http://evil", "\u0000\n")) {
            val url = MusicSearch.searchUrl(MusicSong(t))
            assertNotNull(url)
            assertTrue(url!!.startsWith("https://open.spotify.com/search/"))
            assertNotNull(ExternalUrlPolicy.openable(url))
            assertTrue("区間の外に出ない: $url", url.removePrefix("https://open.spotify.com/search/").none { it in "/?# " })
        }
    }

    @Test
    fun recentKeepsNewestFirstWithoutDuplicates() {
        var recent = emptyList<MusicSong>()
        for (i in 1..7) recent = MusicSearch.remember(recent, MusicSong("曲$i"))
        assertEquals(MusicSearch.MAX_RECENT, recent.size)
        assertEquals("曲7", recent.first().title)
        recent = MusicSearch.remember(recent, MusicSong(" 曲5 ".trim(), ""))
        assertEquals(listOf("曲5", "曲7", "曲6", "曲4", "曲3"), recent.map { it.title })
        assertTrue(MusicSearch.same(MusicSong("Let It Be", "The Beatles"), MusicSong(" let it be", "THE BEATLES ")))
    }
}
