package jp.kz2601.metamon.dsl.logic

import jp.kz2601.metamon.dsl.records.Entity
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.net.URI
import java.net.URLDecoder

/**
 * iOS の GalleryWebTests のうち ShareTarget / ExternalURLPolicy の純粋ロジック。
 * TextHighlight は iOS に単体テストが無いので、Model/TextHighlight.swift の説明どおりの振る舞いを確かめる。
 */
class ShareHighlightTest {
    @Test
    fun shareTargetLenientAndDefaults() {
        assertEquals(ShareTarget.ANY, ShareTarget.of(null))
        assertEquals(ShareTarget.ANY, ShareTarget.of("telegram"))
        assertEquals(ShareTarget.LINE, ShareTarget.of("line"))
        assertEquals("LINE で送る", ShareTarget.LINE.defaultLabel)
    }

    @Test
    fun lineShareUrlEncodesText() {
        val url = URI(ShareTarget.lineShareUrl("今日の記事 https://jphacks.com?a=1&b=2"))
        assertEquals("https", url.scheme)
        assertEquals("line.me", url.host)
        assertEquals("/R/share", url.path)
        val q = url.rawQuery.split("&")
        assertEquals(1, q.size)
        assertEquals("text", q[0].substringBefore("="))
        assertEquals("今日の記事 https://jphacks.com?a=1&b=2", URLDecoder.decode(q[0].substringAfter("="), Charsets.UTF_8))
        assertFalse("& は text の一部としてエンコードされる", url.toString().contains("&b="))
    }

    @Test
    fun channelFormattingIsPlain() {
        val md = "**今日の記事**\n\n\n- 一言: `test`\n# 見出し\n-5度でした"
        val plain = ShareTarget.SLACK.format(md)
        assertEquals("💬 今日の記事\n\n一言: test\n見出し\n-5度でした", plain)
        assertEquals(plain, ShareTarget.DISCORD.format(md))
        assertEquals("LINE はそのまま", md, ShareTarget.LINE.format(md))
        assertEquals(md, ShareTarget.ANY.format(md))
    }

    @Test
    fun externalUrlPolicy() {
        for (ok in listOf("https://a.com", "line://nv/chat", "slack://open", "discord://", "tel:0761234567", "mailto:a@b.c", "maps:?q=x", "comgooglemaps://?q=x", "HTTPS://A.COM")) {
            assertNotNull(ok, ExternalUrlPolicy.openable(ok))
        }
        for (ng in listOf("http://a.com", "ftp://a.com", "javascript:alert(1)", "file:///etc/passwd", "telegram://x", null)) {
            assertNull(ng ?: "nil", ExternalUrlPolicy.openable(ng))
        }
        assertNotNull(ExternalUrlPolicy.webViewUrl("https://jphacks.com/path?x=1"))
        assertNull(ExternalUrlPolicy.webViewUrl("http://jphacks.com"))
        assertNull(ExternalUrlPolicy.webViewUrl("line://nv/chat"))
        assertNull(ExternalUrlPolicy.webViewUrl("https://"))
    }

    // MARK: - text.highlight

    @Test
    fun resolveWordsFromWordsOrField() {
        assertEquals(listOf("りんご", "バナナ"), HighlightWords.resolve(listOf(" りんご ", "", "バナナ", "りんご"), null, null))
        // field の値は「,」「、」改行で区切り、NFKC で全角英数をそろえる。大文字小文字だけ違う語は 1 つ
        assertEquals(listOf("Apple", "みかん", "ぶどう"), HighlightWords.resolve(null, "tags", "Ａｐｐｌｅ、みかん\nぶどう,apple"))
        assertEquals(emptyList<String>(), HighlightWords.resolve(null, "tags", null))
        assertEquals(emptyList<String>(), HighlightWords.resolve(emptyList(), null, "x"))
        val many = (1..30).map { "w$it" }
        assertEquals(HighlightWords.MAX_WORDS, HighlightWords.resolve(many, null, null).size)
        assertEquals(emptyList<String>(), HighlightWords.resolve(listOf("あ".repeat(51)), null, null))
    }

    @Test
    fun rangesFoldWidthAndCaseAndMergeOverlaps() {
        val text = "ＡＰＰＬＥとappleパイ"
        val r = HighlightWords.ranges(text, listOf("apple"))
        assertEquals(listOf(0 until 5, 6 until 11), r)
        // 重なる語はまとめる
        assertEquals(listOf(0 until 5, 6 until 13), HighlightWords.ranges(text, listOf("apple", "pleパイ")))
        assertEquals(emptyList<IntRange>(), HighlightWords.ranges("", listOf("a")))
        assertEquals(emptyList<IntRange>(), HighlightWords.ranges("abc", emptyList()))
    }

    @Test
    fun rangesDoNotSplitGraphemes() {
        // 「が」を濁点の結合文字で書いた文: 「か」で検索しても書記素ごと印をつける
        val text = "がき"
        assertEquals(listOf(0 until 2), HighlightWords.ranges(text, listOf("が")))
    }

    @Test
    fun substitutesOnlyPlainVisibleFields() {
        val e = Entity.parse(
            """{"id":"q","name":"問題","fields":[
              {"id":"text","name":"問題文","type":"string"},
              {"id":"answer","name":"答え","type":"string","hidden":true},
              {"id":"photo","name":"写真","type":"photo"},
              {"id":"due","name":"期限","type":"date"}]}""",
        )
        assertTrue(HighlightWords.substitutes(e, "text"))
        assertTrue(HighlightWords.substitutes(e, "due"))
        assertFalse(HighlightWords.substitutes(e, "answer"))
        assertFalse(HighlightWords.substitutes(e, "photo"))
        assertFalse(HighlightWords.substitutes(e, "nope"))
        assertFalse(HighlightWords.substitutes(null, "text"))
    }
}
