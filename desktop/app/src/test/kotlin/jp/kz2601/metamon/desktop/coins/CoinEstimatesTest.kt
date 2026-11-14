package jp.kz2601.metamon.desktop.coins

import jp.kz2601.metamon.desktop.i18n.Lang
import jp.kz2601.metamon.desktop.i18n.Strings
import kotlinx.serialization.json.Json
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** 送る前の「約 N コイン」の目安の読み方と文の規則(iOS の CoinEstimates・CoinCopy と同じ)。 */
class CoinEstimatesTest {
    private fun read(json: String) = CoinEstimates.fromJson(Json.parseToJsonElement(json))

    @Test
    fun keysIgnoreCaseUnderscoreAndHyphen() {
        val e = read("""{"Generate":30,"ai_button":1,"EDIT":10}""")
        assertEquals(30, e.coins(CoinOperation.GENERATE))
        assertEquals(10, e.coins(CoinOperation.EDIT))
        assertEquals(1, e.coins(CoinOperation.TEXT))
        assertEquals(30, CoinEstimates(mapOf("CREATE" to 30)).coins(CoinOperation.GENERATE))
        assertEquals(2, CoinEstimates(mapOf("ai-chat" to 2)).coins(CoinOperation.TEXT))
    }

    @Test
    fun objectFormsAndStringNumbersAreRead() {
        val e = read("""{"generate":{"coins":30},"edit":{"median":10},"chat":"2","text":{"x":1}}""")
        assertEquals(30, e.coins(CoinOperation.GENERATE))
        assertEquals(10, e.coins(CoinOperation.EDIT))
        assertEquals(2, e.coins(CoinOperation.CHAT))
        assertNull(e.coins(CoinOperation.TEXT))
    }

    @Test
    fun missingZeroAndUnreadableGiveNothing() {
        val e = read("""{"generate":0,"edit":-3,"chat":true,"text":null}""")
        assertNull(e.coins(CoinOperation.GENERATE))
        assertNull(e.coins(CoinOperation.EDIT))
        assertNull(e.coins(CoinOperation.CHAT))
        assertNull(e.coins(CoinOperation.TEXT))
        assertTrue(CoinEstimates.fromJson(null).isEmpty)
        assertTrue(read("[1,2]").isEmpty)
        // 古い名前が 0 でも次の名前を引く
        assertEquals(5, CoinEstimates(mapOf("generate" to 0, "create" to 5)).coins(CoinOperation.GENERATE))
    }

    @Test
    fun firstOfDuplicateKeysWins() {
        assertEquals(30, CoinEstimates(linkedMapOf("generate" to 30, "GENERATE" to 99)).coins(CoinOperation.GENERATE))
    }

    @Test
    fun estimateTextAppearsOnlyWhenEnabledAndKnown() = Strings.withLang(Lang.JA) {
        val e = CoinEstimates(mapOf("generate" to 30))
        assertEquals("約 30 コイン", CoinCopy.estimate(CoinOperation.GENERATE, e, enabled = true))
        assertNull(CoinCopy.estimate(CoinOperation.GENERATE, e, enabled = false))
        assertNull(CoinCopy.estimate(CoinOperation.EDIT, e, enabled = true))
        assertNull(CoinCopy.estimate(CoinOperation.GENERATE, CoinEstimates(), enabled = true))
    }

    @Test
    fun textsInJapaneseAndEnglish() {
        Strings.withLang(Lang.JA) {
            assertEquals("送ると約 30 コインを使います", CoinCopy.estimateAccessibility(30, null))
            assertEquals("送ると約 30 コインを使います。残り 120 コイン", CoinCopy.estimateAccessibility(30, 120))
        }
        Strings.withLang(Lang.EN) {
            val e = CoinEstimates(mapOf("chat" to 2))
            assertEquals("About 2 coins", CoinCopy.estimate(CoinOperation.CHAT, e, enabled = true))
            assertEquals("Sending uses about 30 coins", CoinCopy.estimateAccessibility(30, null))
            assertEquals("Sending uses about 30 coins. 120 coins left", CoinCopy.estimateAccessibility(30, 120))
        }
    }
}
