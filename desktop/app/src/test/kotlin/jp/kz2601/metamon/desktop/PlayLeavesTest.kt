package jp.kz2601.metamon.desktop

import com.google.zxing.BinaryBitmap
import com.google.zxing.RGBLuminanceSource
import com.google.zxing.common.HybridBinarizer
import com.google.zxing.qrcode.QRCodeReader
import jp.kz2601.metamon.dsl.logic.BracketDraw
import jp.kz2601.metamon.dsl.logic.FlashcardDeck
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.screen
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** 計算・遊びの部品(#1745)のうち、適合テストの case に無い操作(書き込み・QR・時間の表示)。 */
class PlayLeavesTest {
    private val entrants = """{"entrant": [
        {"id": "p1", "data": {"name": "青木", "seed": 1, "wins": 1}, "createdAt": "2026-11-01T09:00:00+09:00"},
        {"id": "p2", "data": {"name": "井上", "seed": 2, "wins": 1}, "createdAt": "2026-11-01T09:01:00+09:00"},
        {"id": "p3", "data": {"name": "上田", "wins": 0}, "createdAt": "2026-11-01T09:02:00+09:00"},
        {"id": "p4", "data": {"name": "江藤", "wins": 1}, "createdAt": "2026-11-01T09:03:00+09:00"},
        {"id": "p5", "data": {"name": "小川"}, "createdAt": "2026-11-01T09:04:00+09:00"},
        {"id": "p6", "data": {"name": "加藤", "seed": 3}, "createdAt": "2026-11-01T09:05:00+09:00"}
    ]}"""

    @Test
    fun bracketTapSavesWins() {
        val dsl = Examples.load("tournament")
        val session = DesktopSession(dsl, MockStore.fromJson(entrants))
        val c = dsl.screen("home")!!.allComponents().first { it.id == "draw" }
        val draw = PlayRules.draw(c, session.leafData(), session.records("entrant"), emptyMap())
        val open = draw.rounds[0][3]
        assertNull(open.winner)
        val wins = PlayRules.winsField(c, session.entityModel("entrant"))!!
        val update = draw.tap(open, BracketDraw.Side.B, wins) as BracketDraw.TapResult.Update
        PlayRules.writePlan(update.wins, emptyMap(), wins, null).forEach { (id, patch) -> assertTrue(session.updateRecord("entrant", id, patch)) }
        val after = PlayRules.draw(c, session.leafData(), session.records("entrant"), emptyMap())
        assertEquals(BracketDraw.Side.B, after.rounds[0][3].winner)
        assertEquals("小川", PlayRules.entrantName(c, session.leafData(), after.rounds[0][3].winnerRecord!!))
        // 不戦勝の試合は選べない
        assertEquals(BracketDraw.TapResult.NotPlayable, after.tap(after.rounds[0][0], BracketDraw.Side.A, wins))
    }

    @Test
    fun flashcardAnswerSavesToggleAndLeavesDeck() {
        val dsl = Examples.load("vocab")
        val store = MockStore.fromJson(
            """{"word": [{"id": "w1", "data": {"term": "apple", "meaning": "りんご", "learned": false}, "createdAt": "2026-11-01T09:00:00+09:00"},
                {"id": "w3", "data": {"term": "consider", "meaning": "よく考える"}, "createdAt": "2026-11-03T09:00:00+09:00"}]}""",
        )
        val session = DesktopSession(dsl, store)
        val c = dsl.screen("home")!!.allComponents().first { it.id == "cards" }
        val deck = FlashcardDeck()
        val cards = PlayRules.cards(c, session.leafData(), deck.pending)
        assertEquals(listOf("w1", "w3"), cards.map { it.id })
        val toggle = PlayRules.toggleField(c, session.entityModel("word"))
        val patch = deck.answer(cards[0], learned = true, toggle = toggle)!!
        assertTrue(session.updateRecord("word", "w1", patch))
        assertEquals(JSONValue.Bool(true), session.records("word").first { it.id == "w1" }["learned"])
        // 覚えたカードは filter(learned = false)で山札から外れる
        assertEquals(listOf("w3"), PlayRules.cards(c, session.leafData(), deck.pending).map { it.id })
    }

    @Test
    fun updateRecordRemovesNullValues() {
        val dsl = Examples.load("tournament")
        val session = DesktopSession(dsl, MockStore.fromJson(entrants))
        assertTrue(session.updateRecord("entrant", "p1", mapOf("started" to JSONValue.Str("2026-11-12T12:00:00+09:00"))))
        assertEquals("2026-11-12T12:00:00+09:00", session.store.find("entrant", "p1")!!.data["started"])
        assertTrue(session.updateRecord("entrant", "p1", mapOf("started" to JSONValue.Null)))
        assertFalse(session.store.find("entrant", "p1")!!.data.containsKey("started"))
        assertFalse(session.updateRecord("entrant", "missing", mapOf("wins" to JSONValue.Num(1.0))))
    }

    @Test
    fun qrMatrixRoundTrips() {
        for (text in listOf("https://example.com", "こんにちは")) {
            val m = QrMatrix.encode(text)!!
            val px = IntArray(m.size * m.size) { i -> if (m[i % m.size, i / m.size]) 0xFF000000.toInt() else 0xFFFFFFFF.toInt() }
            val bitmap = BinaryBitmap(HybridBinarizer(RGBLuminanceSource(m.size, m.size, px)))
            assertEquals(text, QRCodeReader().decode(bitmap, mapOf(com.google.zxing.DecodeHintType.PURE_BARCODE to true)).text)
        }
        assertEquals(25 + 2 * QrMatrix.QUIET_ZONE, QrMatrix.encode("https://example.com")!!.size)
        assertNull(QrMatrix.encode(""))
        assertNull(QrMatrix.encode("x".repeat(5000)))
    }

    @Test
    fun timerFormatsAndCapsCountdown() {
        assertEquals("05:03", TimerRun.format(303))
        assertEquals("1:02:03", TimerRun.format(3723))
        val run = TimerRun(base = 10, startedAtMillis = 1_000L)
        assertEquals(15, run.elapsed(6_000L))
        assertEquals(12, run.elapsed(6_000L, total = 12))
        assertEquals(1_000L + 50_000L, run.endsAtMillis(60))
        assertNull(TimerRun(base = 3).endsAtMillis(60))
    }

    @Test
    fun settlementGreedyTransfers() {
        val dsl = Examples.load("warikan")
        val session = DesktopSession(dsl)
        val c = dsl.screens.flatMap { it.allComponents() }.first { it.type == jp.kz2601.metamon.dsl.ComponentType.SETTLEMENT }
        assertTrue(PlayRules.transfers(c, session.leafData()).isEmpty())
    }
}
