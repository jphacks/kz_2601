package jp.kz2601.metamon.dsl.ai

import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.decode
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.entityModels
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.jsonObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

/** aiButton の依頼と答えの形(iOS の AITextTests / AISaveToTests と同じ規則)。 */
class AiTextTest {
    private fun req(shape: AiTextShape = AiTextShape.LINE, count: Int? = null, items: List<String>? = null, task: String? = null, options: List<String>? = null, length: String? = null) =
        AiTextRequest(instruction = "書いて", shape = shape, count = count, items = items, task = task, options = options, length = length)

    @Test
    fun lineTakesFirstNonEmptyLineAndDropsMarks() {
        val r = AiTextResult.normalize("\n  - 「今日もいい日」\n次の行", null, null, req())
        assertEquals(AiTextResult(AiTextShape.LINE, "今日もいい日"), r)
        assertNull(AiTextResult.normalize("   \n ", null, null, req()))
    }

    @Test
    fun listCutsToCountAndJoinsWithDots() {
        val r = AiTextResult.normalize(null, listOf("1. 案A", "", "・案B", "案C"), null, req(AiTextShape.LIST, count = 2))!!
        assertEquals(listOf("案A", "案B"), r.items)
        assertEquals("・案A\n・案B", r.text)
        assertNull(AiTextResult.normalize(null, listOf(" "), null, req(AiTextShape.LIST)))
    }

    @Test
    fun fieldsNeedEveryRequestedNameInRequestOrder() {
        val request = req(AiTextShape.FIELDS, items = listOf("タイトル", "本文"))
        val r = AiTextResult.normalize(null, null, listOf(AiTextField("本文", "中身"), AiTextField("タイトル", "題")), request)!!
        assertEquals(listOf("タイトル", "本文"), r.fields!!.map { it.name })
        assertEquals("タイトル: 題\n本文: 中身", r.text)
        assertNull(AiTextResult.normalize(null, null, listOf(AiTextField("タイトル", "題")), request))
    }

    @Test
    fun classifyAcceptsOnlyOneMatchingOption() {
        val request = req(task = "classify", options = listOf("嬉しい", "悲しい"))
        assertEquals("嬉しい", AiTextResult.normalize("「嬉しい」", null, null, request)!!.text)
        assertEquals("悲しい", AiTextResult.normalize("たぶん悲しいです", null, null, request)!!.text)
        assertNull(AiTextResult.normalize("嬉しいし悲しい", null, null, request))
        assertNull(AiTextResult.normalize("怒り", null, null, request))
    }

    @Test
    fun transformKeepsWholeTextAndLongKeepsParagraphs() {
        val sum = AiTextResult.normalize("1 行目\n2 行目", null, null, req(task = "summarize"))!!
        assertEquals("1 行目\n2 行目", sum.text)
        assertEquals(AiTextLimits.MAX_TASK_CHARACTERS, AiTextResult.normalize("あ".repeat(2000), null, null, req(task = "summarize"))!!.text.length)
        val long = AiTextResult.normalize("段落 1\n\n段落 2", null, null, req(length = "long"))!!
        assertEquals("段落 1\n\n段落 2", long.text)
    }

    @Test
    fun clipNeverSplitsACharacter() {
        assertEquals("abc", AiTextLimits.clip("abc", 4))
        assertEquals("あ", AiTextLimits.clip("あい", 5)) // 3 + 3 バイト
        assertEquals("a😀", AiTextLimits.clip("a😀b", 5))
    }

    @Test
    fun requestBodyOmitsNullsAndIsTransformOnlyForTasks() {
        val json = Json { explicitNulls = false; encodeDefaults = true }
        val body = json.parseToJsonElement(json.encodeToString(AiTextRequest.serializer(), req(task = "summarize").inApp(""))).jsonObject
        assertEquals(setOf("instruction", "shape", "task"), body.keys)
        assertTrue(req(task = "summarize").isTransform)
        assertFalse(req(task = "generate").isTransform)
        assertEquals("app1", req().inApp("app1").appId)
        assertEquals(10, req(AiTextShape.LIST, count = 99).resolvedCount)
    }

    @Test
    fun placeholderReplacesOnceAndKeepsUnknownIds() {
        assertEquals(setOf("input", "mood"), AiPlaceholder.ids("{{ input }} と {{mood}}"))
        assertEquals("X と {{other}}", AiPlaceholder.replace("{{input}} と {{other}}") { if (it == "input") "X" else null })
        assertEquals("{{input}}", AiPlaceholder.replace("{{a}}") { "{{input}}" })
    }

    private val ideas = AppDSL.decode(
        """
        {"schemaVersion":1,"app":{"name":"t","voiceEntry":"none"},"initialScreen":"home",
         "entities":[{"id":"idea","name":"案","fields":[
           {"id":"title","name":"案","type":"string","required":true},
           {"id":"done","name":"済み","type":"boolean"},
           {"id":"count","name":"数","type":"integer"}]}],
         "screens":[{"id":"home","title":"t","components":[]}]}
        """.trimIndent(),
    ).entityModels().single()

    @Test
    fun saveToRecordPutsItemInFieldAndDefaultsElsewhere() {
        val saveTo = AiSaveTo.from(Json.parseToJsonElement("""{"entity":"idea","field":"title"}"""))!!
        val data = saveTo.record("  変顔で自撮り  ", ideas)!!
        assertEquals(JSONValue.Str("変顔で自撮り"), data["title"])
        assertEquals(JSONValue.Bool(false), data["done"])
        assertNull(saveTo.record("  ", ideas))
        assertNull(AiSaveTo("idea", "done").record("x", ideas)) // string / text でない
        assertNull(AiSaveTo("other", "title").record("x", ideas))
        assertNull(AiSaveTo.from(JsonObject(emptyMap())))
    }

    @Test
    fun roomStopsAtEntityLimit() {
        assertEquals(3, AiSaveTo.room(existing = 0, wanted = 3))
        assertEquals(1, AiSaveTo.room(existing = 1999, wanted = 3))
        assertEquals(0, AiSaveTo.room(existing = 2500, wanted = 3))
    }
}
