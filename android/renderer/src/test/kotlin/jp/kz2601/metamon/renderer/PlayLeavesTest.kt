package jp.kz2601.metamon.renderer

import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.dsl.decode
import jp.kz2601.metamon.dsl.logic.BracketDraw
import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.dsl.records.entityModels
import jp.kz2601.metamon.dsl.screen
import jp.kz2601.metamon.renderer.components.lists.LeafData
import jp.kz2601.metamon.renderer.components.play.BracketRules
import jp.kz2601.metamon.renderer.components.play.FlashcardRules
import jp.kz2601.metamon.renderer.components.play.PlaySave
import jp.kz2601.metamon.renderer.components.play.RecordWriter
import jp.kz2601.metamon.renderer.components.play.TimerRun
import jp.kz2601.metamon.renderer.components.play.randomPickerSubtitle
import jp.kz2601.metamon.renderer.components.play.randomPickerTitle
import jp.kz2601.metamon.renderer.components.play.settlementTransfers
import jp.kz2601.metamon.renderer.registry.ComponentRegistry
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.runBlocking
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** 計算・遊び系の Leaf 5 種(#860)の値の決め方(iOS の SettlementLeaf / RandomPickerLeaf / BracketLeaf / FlashcardsLeaf / TimerDisplayLeaf)。 */
class PlayLeavesTest {
    private val dsl = AppDSL.decode(
        """
        {"schemaVersion":1,"app":{"name":"t","voiceEntry":"none"},"initialScreen":"s",
         "entities":[
          {"id":"member","name":"メンバー","titleField":"name","fields":[
            {"id":"name","name":"名前","type":"string"},
            {"id":"paid","name":"払った","type":"currency"},
            {"id":"share","name":"負担","type":"currency"},
            {"id":"balance","name":"差額","type":"currency","computed":{"op":"sub","args":[{"ref":"paid"},{"ref":"share"}]}}
          ]},
          {"id":"player","name":"出場者","titleField":"name","fields":[
            {"id":"name","name":"名前","type":"string"},
            {"id":"wins","name":"勝ち","type":"integer"},
            {"id":"seed","name":"シード","type":"integer"},
            {"id":"started","name":"開始","type":"datetime"},
            {"id":"score","name":"点","type":"integer","computed":{"op":"add","args":[{"ref":"wins"},1]}}
          ]},
          {"id":"card","name":"カード","fields":[
            {"id":"word","name":"単語","type":"string"},
            {"id":"meaning","name":"意味","type":"string"},
            {"id":"learned","name":"覚えた","type":"boolean"},
            {"id":"level","name":"レベル","type":"integer"}
          ]}
         ],
         "screens":[{"id":"s","title":"s","components":[
          {"type":"settlement","entity":"member","nameField":"name","balanceField":"balance"},
          {"type":"randomPicker","entity":"player","titleField":"name","subtitleField":"seed","filter":{"field":"seed","op":"lte","value":2}},
          {"type":"bracket","entity":"player","titleField":"name","winsField":"wins","seedField":"seed","startedField":"started"},
          {"type":"flashcards","entity":"card","front":"word","back":"meaning","toggle":"learned","filter":{"field":"learned","op":"eq","value":false}},
          {"type":"timerDisplay","mode":"countdown","seconds":90,"field":"t"},
          {"type":"bracket","entity":"player","winsField":"score","startedField":"name"},
          {"type":"flashcards","entity":"card","front":"word","back":"meaning","toggle":"level"}
         ]}]}
        """.trimIndent(),
    )
    private val entities = dsl.entityModels()
    private val components: List<Component> = dsl.screen("s")!!.components
    private fun comp(i: Int) = components[i]

    private class FakeRuntime(
        override val entityModels: List<Entity>,
        records: Map<String, List<Record>> = emptyMap(),
        private val editable: Boolean = true,
    ) : DSLRuntime {
        override val recordsFlow: StateFlow<Map<String, List<Record>>> = MutableStateFlow(records)
        override fun onVoiceInput(componentId: String?) = Unit
        override fun canEdit(entity: String, record: Record) = editable
    }

    private fun rec(id: String, vararg data: Pair<String, JSONValue>, createdAt: String? = null) =
        Record(id, mapOf(*data), createdAt = createdAt)

    private fun s(v: String) = JSONValue.Str(v)
    private fun n(v: Double) = JSONValue.Num(v)

    @Test
    fun registryHasTheFivePlayLeaves() {
        val wires = listOf(ComponentType.SETTLEMENT, ComponentType.RANDOM_PICKER, ComponentType.BRACKET, ComponentType.FLASHCARDS, ComponentType.TIMER_DISPLAY)
            .map { it.wire }
        assertTrue(ComponentRegistry.standard.registeredTypes.containsAll(wires))
    }

    @Test
    fun settlementUsesComputedBalanceAndNames() {
        val members = listOf(
            rec("a", "name" to s("A"), "paid" to n(3000.0), "share" to n(1000.0)),
            rec("b", "name" to s("B"), "paid" to n(0.0), "share" to n(1000.0)),
            rec("c", "name" to s("C"), "paid" to n(0.0), "share" to n(1000.0)),
        )
        val all = mapOf("member" to members)
        val rt = FakeRuntime(entities, all)
        val t = settlementTransfers(comp(0), rt, all)
        assertEquals(2, t.size)
        assertEquals(setOf("B", "C"), t.map { it.from }.toSet())
        assertTrue(t.all { it.to == "A" && it.amount == 1000.0 })
    }

    @Test
    fun settlementIsEmptyWhenEveryoneIsEven() {
        val all = mapOf("member" to listOf(rec("a", "name" to s("A"), "paid" to n(500.0), "share" to n(500.0))))
        assertTrue(settlementTransfers(comp(0), FakeRuntime(entities, all), all).isEmpty())
    }

    @Test
    fun randomPickerFiltersCandidatesAndReadsTitles() {
        val players = listOf(
            rec("p1", "name" to s("一郎"), "seed" to n(1.0)),
            rec("p2", "name" to s(""), "seed" to n(2.0)),
            rec("p3", "name" to s("三郎"), "seed" to n(3.0)),
        )
        val all = mapOf("player" to players)
        val rt = FakeRuntime(entities, all)
        val c = comp(1)
        val candidates = LeafData(rt, all).filtered(c)
        assertEquals(listOf("p1", "p2"), candidates.map { it.id })
        assertEquals("一郎", randomPickerTitle(c, rt, all, players[0]))
        assertNull("空の見出しは呼び側が「(無題)」", randomPickerTitle(c, rt, all, players[1]))
        assertEquals("1", randomPickerSubtitle(c, rt, all, players[0]))
    }

    @Test
    fun bracketDrawsRoundsAndRoundTitles() {
        val players = (1..4).map { rec("p$it", "name" to s("P$it"), "seed" to n(it.toDouble()), createdAt = "2026-11-0${it}T00:00:00Z") }
        val all = mapOf("player" to players)
        val draw = BracketRules.draw(comp(2), FakeRuntime(entities, all), all, emptyMap())
        assertEquals(2, draw.rounds.size)
        assertEquals("p1", draw.rounds[0][0].a?.id)
        assertEquals("p4", draw.rounds[0][0].b?.id)
        assertEquals("準決勝", BracketRules.roundTitle(1, 2))
        assertEquals("決勝", BracketRules.roundTitle(2, 2))
        assertEquals("準々決勝", BracketRules.roundTitle(1, 3))
        assertEquals("1 回戦", BracketRules.roundTitle(1, 4))
        assertEquals("P1", BracketRules.name(comp(2), FakeRuntime(entities, all), all, players[0]))
    }

    @Test
    fun bracketWritesOnlyToStoredIntegerAndDatetimeFields() {
        val player = entities.first { it.id == "player" }
        assertEquals("wins", BracketRules.winsField(comp(2), player))
        assertEquals("started", BracketRules.startedField(comp(2), player))
        assertNull("computed は書かない", BracketRules.winsField(comp(5), player))
        assertNull("datetime でない", BracketRules.startedField(comp(5), player))
    }

    @Test
    fun bracketWritePlanLowersFirstAndStampsStartBeforeWins() {
        val wins = mapOf("b" to 1, "a" to 0)
        assertEquals(listOf("a" to 0, "b" to 1), BracketRules.writeOrder(wins))
        val now = s("2026-11-11T00:00:00Z")
        val starts = mapOf("b" to now, "c" to now)
        val plan = BracketRules.writePlan(wins, starts, "wins", "started")
        assertEquals(listOf("c", "a", "b"), plan.map { it.first })
        assertEquals(mapOf("wins" to n(1.0), "started" to now), plan[2].second)
        // 取り消し(開始時刻を消す)は勝ち数を戻してから
        val clear = BracketRules.writePlan(mapOf("b" to 0), mapOf("b" to JSONValue.Null, "c" to JSONValue.Null), "wins", "started")
        assertEquals(listOf("b", "c"), clear.map { it.first })
    }

    @Test
    fun bracketTapThenSaveInOrderStopsAfterFailure() = runBlocking {
        val players = (1..2).map { rec("p$it", "name" to s("P$it"), "seed" to n(it.toDouble())) }
        val all = mapOf("player" to players)
        val draw = BracketRules.draw(comp(2), FakeRuntime(entities, all), all, emptyMap())
        val result = draw.tap(draw.rounds[0][0], BracketDraw.Side.A, "wins") as BracketDraw.TapResult.Update
        assertEquals(mapOf("p1" to 1), result.wins)

        val sent = mutableListOf<String>()
        val settled = mutableListOf<String>()
        val writer = object : RecordWriter {
            override suspend fun updateRecord(entity: String, recordId: String, data: Map<String, JSONValue>): Boolean {
                sent += recordId
                return recordId != "x"
            }
        }
        val ok = PlaySave.updateInOrder(writer, "player", listOf("x" to emptyMap(), "y" to emptyMap()), settled::add)
        assertFalse(ok)
        assertEquals(listOf("x"), sent)
        assertEquals(listOf("x", "y"), settled)
    }

    @Test
    fun flashcardsTreatMissingToggleAsNotYetAndKeepCreationOrder() {
        val cards = listOf(
            rec("c2", "word" to s("dog"), "meaning" to s("犬"), createdAt = "2026-11-02T00:00:00Z"),
            rec("c1", "word" to s("cat"), "meaning" to s("猫"), "learned" to JSONValue.Bool(false), createdAt = "2026-11-01T00:00:00Z"),
            rec("c3", "word" to s("owl"), "learned" to JSONValue.Bool(true), createdAt = "2026-11-03T00:00:00Z"),
        )
        val all = mapOf("card" to cards)
        val rt = FakeRuntime(entities, all)
        val c = comp(3)
        assertEquals(false, FlashcardRules.learnedFilter(c))
        assertEquals(listOf("c1", "c2"), FlashcardRules.cards(c, rt, all, emptyMap()).map { it.id })
        // 保存待ちの「覚えた」は山札から外れる
        assertEquals(listOf("c2"), FlashcardRules.cards(c, rt, all, mapOf("c1" to true)).map { it.id })
        val card = entities.first { it.id == "card" }
        assertEquals("learned", FlashcardRules.toggleField(c, card))
        assertNull("boolean でない toggle はめくるだけ", FlashcardRules.toggleField(comp(6), card))
        assertEquals("—", FlashcardRules.value(rt, all, card, cards[2], "meaning"))
    }

    @Test
    fun timerRunComputesFromStartTimeAndCapsCountdown() {
        val run = TimerRun(base = 10, startedAtMillis = 1_000L)
        assertTrue(run.isRunning)
        assertEquals(15, run.elapsed(6_000L))
        assertEquals(12, run.elapsed(6_000L, total = 12))
        assertEquals(1_000L + 80_000L, run.endsAtMillis(90))
        assertEquals(81_000L, TimerRun.notificationAt(true, run, 90, nowMillis = 6_000L))
        assertNull("stopwatch は通知しない", TimerRun.notificationAt(false, run, 90, nowMillis = 6_000L))
        assertNull("終わった後は取り消す", TimerRun.notificationAt(true, run, 90, nowMillis = 90_000L))
        assertNull("止まっていれば取り消す", TimerRun.notificationAt(true, TimerRun(base = 5), 90, nowMillis = 0L))
        assertEquals("01:30", TimerRun.format(90))
        assertEquals("1:00:05", TimerRun.format(3605))
    }
}
