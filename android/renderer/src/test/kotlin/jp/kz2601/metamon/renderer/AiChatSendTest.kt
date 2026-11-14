package jp.kz2601.metamon.renderer

import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.dsl.DslJson
import jp.kz2601.metamon.dsl.ai.AiTextError
import jp.kz2601.metamon.dsl.ai.AiTextRequest
import jp.kz2601.metamon.dsl.ai.AiTextResult
import jp.kz2601.metamon.dsl.ai.AiTextShape
import jp.kz2601.metamon.dsl.decode
import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.dsl.records.entityModels
import jp.kz2601.metamon.renderer.components.ai.AiChatMic
import jp.kz2601.metamon.renderer.components.ai.AiChatSend
import jp.kz2601.metamon.renderer.components.ai.AiChatSuggestion
import jp.kz2601.metamon.renderer.components.ai.AiHost
import jp.kz2601.metamon.renderer.components.ai.DictationPhase
import jp.kz2601.metamon.renderer.components.ai.DictationReadiness
import jp.kz2601.metamon.renderer.components.ai.appendDictation
import jp.kz2601.metamon.renderer.registry.ComponentRegistry
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import kotlinx.coroutines.runBlocking
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** aiChat(#874): 送る・対で保存・記録の案・エラーの文言・マイクの状態(iOS の AiChatLeafTests)。live の AI は呼ばない。 */
class AiChatSendTest {
    private val dsl = AppDSL.decode(
        """
        {"schemaVersion":1,"app":{"name":"t","voiceEntry":"none"},"initialScreen":"chat",
         "entities":[
          {"id":"talk","name":"会話","fields":[{"id":"text","name":"本文","type":"text"},{"id":"role","name":"役","type":"string"}]},
          {"id":"meal","name":"食事","fields":[{"id":"food","name":"料理","type":"string","required":true},{"id":"kcal","name":"カロリー","type":"number"}]},
          {"id":"diary","name":"日記","fields":[{"id":"body","name":"本文","type":"text"}]}],
         "screens":[{"id":"chat","title":"t","components":[]}]}
        """.trimIndent(),
    )

    private class FakeHost(var answer: (AiTextRequest) -> AiTextResult) : AiHost {
        val requests = mutableListOf<AiTextRequest>()
        override val supportsOnlineText: Boolean = true
        override suspend fun generateText(request: AiTextRequest): AiTextResult {
            requests += request
            return answer(request)
        }
    }

    private class FakeRuntime(dsl: AppDSL, override val ai: AiHost?) : DSLRuntime {
        override fun onVoiceInput(componentId: String?) = Unit
        override val entityModels: List<Entity> = dsl.entityModels()
        val records = MutableStateFlow<Map<String, List<Record>>>(emptyMap())
        override val recordsFlow: StateFlow<Map<String, List<Record>>> get() = records
        var failAdds = emptySet<Int>()
        var adds = 0
        val loaded = mutableListOf<Set<String>>()

        override suspend fun loadRecords(entities: Set<String>) {
            loaded += entities
        }

        override suspend fun addRecord(entity: String, data: Map<String, JSONValue>): Boolean {
            adds++
            if (adds in failAdds) return false
            val list = records.value[entity].orEmpty()
            val at = "2026-01-01T00:00:%02dZ".format(list.size)
            records.value = records.value + (entity to list + Record("r${list.size}", data, createdAt = at))
            return true
        }
    }

    private fun comp(json: String): Component = DslJson.decodeFromString(Component.serializer(), json)

    private fun chat(extra: String = "") = comp(
        """{"type":"aiChat","id":"c","entity":"talk","textField":"text","roleField":"role","persona":"先生"$extra}""",
    )

    private fun reply(text: String, suggest: JsonObject? = null) = { _: AiTextRequest -> AiTextResult(AiTextShape.LINE, text, suggest = suggest) }

    @Test
    fun registeredAsAiChat() {
        assertNotNull(ComponentRegistry.standard.builder(ComponentType.AI_CHAT))
    }

    @Test
    fun savesMineFirstThenTheReply() = runBlocking {
        val host = FakeHost(reply("こんにちは"))
        val rt = FakeRuntime(dsl, host)
        val spec = AiChatSend.Spec.of(chat())!!
        val out = AiChatSend.send(rt, spec, " やあ ", "ja")
        assertTrue(out is AiChatSend.Outcome.Sent)
        val saved = rt.records.value["talk"].orEmpty()
        assertEquals(listOf("やあ", "こんにちは"), saved.map { it.data["text"]?.stringValue })
        assertEquals(listOf("user", "assistant"), saved.map { it.data["role"]?.stringValue })
        assertEquals("r1", (out as AiChatSend.Outcome.Sent).turnId)
        val req = host.requests.single()
        assertEquals("chat", req.task)
        assertEquals("先生", req.instruction)
        assertEquals("ja", req.locale)
        assertEquals(listOf("やあ"), req.messages!!.map { it.text })
        // 2 回目は前の往復を履歴に送る
        AiChatSend.send(rt, spec, "次", "ja")
        assertEquals(listOf("やあ", "こんにちは", "次"), host.requests.last().messages!!.map { it.text })
    }

    @Test
    fun replyFailureKeepsMyMessageWhenRoleFieldExists() = runBlocking {
        val rt = FakeRuntime(dsl, FakeHost { throw AiTextError.NeedsPro })
        val out = AiChatSend.send(rt, AiChatSend.Spec.of(chat())!!, "やあ", "ja") as AiChatSend.Outcome.Failed
        assertEquals(R.string.ai_chat_needs_pro, out.message.res)
        assertFalse(out.restore)
        assertEquals(1, rt.records.value["talk"]?.size)
    }

    @Test
    fun oldDslSavesThePairOrNothing() = runBlocking {
        val noRole = comp("""{"type":"aiChat","id":"c","entity":"talk","textField":"text"}""")
        val rt = FakeRuntime(dsl, FakeHost(reply("返事")))
        val pending = mutableListOf<String?>()
        val ok = AiChatSend.send(rt, AiChatSend.Spec.of(noRole)!!, "やあ", "ja") { pending += it }
        assertTrue(ok is AiChatSend.Outcome.Sent)
        assertEquals(listOf("やあ", null), pending)
        assertEquals(2, rt.records.value["talk"]?.size)
        assertFalse(rt.records.value["talk"]!!.any { it.data.containsKey("role") })

        // 返事が来なければ何も保存せず、文を入力欄に戻す
        val rt2 = FakeRuntime(dsl, FakeHost { throw AiTextError.CreatorNeedsPro })
        val failed = AiChatSend.send(rt2, AiChatSend.Spec.of(noRole)!!, "やあ", "ja") as AiChatSend.Outcome.Failed
        assertTrue(failed.restore)
        assertEquals(R.string.ai_chat_creator_needs_pro, failed.message.res)
        assertNull(rt2.records.value["talk"])
    }

    @Test
    fun offlineHostDoesNotSend() = runBlocking {
        val rt = FakeRuntime(dsl, null)
        val out = AiChatSend.send(rt, AiChatSend.Spec.of(chat())!!, "やあ", "ja") as AiChatSend.Outcome.Failed
        assertEquals(R.string.ai_chat_offline, out.message.res)
        assertNull(rt.records.value["talk"])
    }

    @Test
    fun saveToMakesASuggestionAndAddSavesIt() = runBlocking {
        val suggest = JsonObject(mapOf("food" to JsonPrimitive("カレー"), "kcal" to JsonPrimitive(800)))
        val host = FakeHost(reply("おいしそう", suggest))
        val rt = FakeRuntime(dsl, host)
        val c = chat(""","saveTo":{"entity":"meal","fields":["food","kcal"]}""")
        val out = AiChatSend.send(rt, AiChatSend.Spec.of(c)!!, "今日カレー食べた", "ja") as AiChatSend.Outcome.Sent
        val req = host.requests.single()
        assertEquals(listOf("food", "kcal"), req.saveTo!!.fields.map { it.id })
        val s = out.suggestion!!
        assertEquals(out.turnId, s.turnId)
        assertEquals(listOf("料理" to "カレー", "カロリー" to "800"), s.rows)
        assertNull(rt.records.value["meal"]) // 押すまで保存しない
        assertNull(AiChatSend.addSuggestion(rt, s))
        assertEquals(JSONValue.Str("カレー"), rt.records.value["meal"]!!.single().data["food"])
    }

    @Test
    fun suggestionNeedsRequiredFields() {
        val meal = dsl.entityModels().first { it.id == "meal" }
        val saveTo = jp.kz2601.metamon.dsl.AIChatSaveTo("meal", listOf("food", "kcal"))
        assertNull(AiChatSuggestion.make("t", mapOf("kcal" to JSONValue.Num(1.0)), saveTo, meal) { v, _ -> v.displayString })
    }

    @Test
    fun inputEntityAddsOwnRecordsAsMaterial() = runBlocking {
        val host = FakeHost(reply("なるほど"))
        val rt = FakeRuntime(dsl, host)
        rt.records.value = mapOf("diary" to listOf(Record("d1", mapOf("body" to JSONValue.Str("散歩した")), createdAt = "2026-01-01T00:00:00Z")))
        AiChatSend.send(rt, AiChatSend.Spec.of(chat(""","inputEntity":"diary","limit":5"""))!!, "最近どう?", "ja")
        val m = host.requests.single().material!!
        assertEquals("diary", m.entity)
        assertEquals("- 本文: 散歩した", m.text)
        assertTrue(setOf("diary") in rt.loaded)
    }

    @Test
    fun micStates() {
        assertEquals(AiChatMic.HIDDEN, AiChatMic.state(DictationPhase.IDLE, null, false, true))
        assertEquals(AiChatMic.START, AiChatMic.state(DictationPhase.IDLE, DictationReadiness.READY, false, true))
        assertEquals(AiChatMic.BUSY, AiChatMic.state(DictationPhase.IDLE, DictationReadiness.READY, true, true))
        assertEquals(AiChatMic.BUSY, AiChatMic.state(DictationPhase.IDLE, DictationReadiness.READY, false, false))
        assertEquals(AiChatMic.PREPARE, AiChatMic.state(DictationPhase.IDLE, DictationReadiness.NEEDS_MODEL, false, true))
        assertEquals(AiChatMic.STOP, AiChatMic.state(DictationPhase.LISTENING, DictationReadiness.READY, false, true))
        assertFalse(AiChatMic.canSend(true, false, "やあ", DictationPhase.LISTENING))
        assertTrue(AiChatMic.canSend(true, false, "やあ", DictationPhase.IDLE))
        assertFalse(AiChatMic.canSend(true, false, "  ", DictationPhase.IDLE))
        assertEquals("前\n聞いた", appendDictation(" 聞いた ", "前"))
        assertEquals("聞いた", appendDictation("聞いた", ""))
    }
}
