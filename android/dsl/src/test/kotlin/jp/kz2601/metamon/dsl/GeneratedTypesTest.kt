package jp.kz2601.metamon.dsl

import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.doubleOrNull
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.io.File

/** generated/(backend/cmd/dslgen の出力)が正典の例をこぼさず読めるか・新しい版の値で落ちないか。 */
class GeneratedTypesTest {
    private val examples: List<File> = File(
        requireNotNull(System.getProperty("metamon.dslExamplesDir")) { "metamon.dslExamplesDir が未設定(Gradle から実行する)" },
    ).listFiles { f -> f.extension == "json" }.orEmpty().sortedBy { it.name }

    // ---- 例を全部 ----------------------------------------------------------

    /** 既定値(required の enum など)も書き出して、元の JSON と比べられるようにする。 */
    private val encoder = kotlinx.serialization.json.Json(DslJson) { encodeDefaults = true }

    /** 読んで書き戻すと同じ JSON になる = 例のキー・値を 1 つも落としていない(型の抜け・知らない enum の検出)。 */
    @Test
    fun everyExampleRoundTripsWithoutLosingKeys() {
        assertTrue(examples.isNotEmpty())
        for (file in examples) {
            val original = DslJson.parseToJsonElement(file.readText())
            val decoded = DslJson.decodeFromJsonElement(AppDSL.serializer(), original)
            val encoded = encoder.encodeToJsonElement(AppDSL.serializer(), decoded)
            val diff = firstDiff("$", normalize(original), normalize(encoded))
            assertNull("${file.name}: 読み書きで値が変わった $diff", diff)
        }
    }

    /** 例の部品は、その部品に書いてよいキー(DslKeys)だけを使う。 */
    @Test
    fun everyExampleUsesOnlyKeysAllowedForItsComponent() {
        for (file in examples) {
            val root = DslJson.parseToJsonElement(file.readText()).jsonObject
            for (screen in root["screens"]?.jsonArray.orEmpty()) {
                for (component in allComponents(screen.jsonObject["components"] as? JsonArray ?: JsonArray(emptyList()))) {
                    val type = DslJson.decodeFromJsonElement(ComponentType.serializer(), component["type"] ?: JsonNull)
                    val allowed = requireNotNull(DslKeys.component[type]) { "${file.name}: $type の表が無い" }
                    val extra = component.keys - allowed
                    assertTrue("${file.name}: ${type.wire} に書けないキー $extra", extra.isEmpty())
                }
            }
        }
    }

    @Test
    fun keyTablesCoverEveryType() {
        assertEquals(ComponentType.entries.toSet() - ComponentType.UNKNOWN, DslKeys.component.keys)
        assertEquals(ActionType.entries.toSet() - ActionType.UNKNOWN, DslKeys.action.keys)
        assertTrue(DslKeys.action.getValue(ActionType.NAVIGATE).contains("screen"))
        assertTrue(!DslKeys.action.getValue(ActionType.BACK).contains("screen"))
    }

    // ---- 新しい版の値・壊れた値 --------------------------------------------

    @Test
    fun unknownEnumValuesFallBack() {
        val dsl = decode(
            """{"id":"x","type":"voiceInput","size":"huge","spacing":"tight"},
               {"type":"button","label":"b","action":{"type":"teleport","screen":"s"}}""",
            app = """{"name":"a","voiceEntry":"none","theme":{"mode":"sepia"}}""",
        )
        val (mic, button) = dsl.screens.single().components
        assertEquals(Size.UNKNOWN, mic.size)
        assertEquals(Spacing.UNKNOWN, mic.spacing)
        assertEquals(ThemeMode.UNKNOWN, dsl.app.theme?.mode)
        assertEquals(ActionType.UNKNOWN, button.action?.type)
        assertEquals("s", button.action?.screen)
        assertEquals("teleport", button.action?.rawType)
    }

    @Test
    fun actionKeepsRawTypeForKnownTypes() {
        val dsl = decode("""{"type":"button","label":"b","action":{"type":"navigate","screen":"s"}}""")
        val action = dsl.screens.single().components.single().action
        assertEquals(ActionType.NAVIGATE, action?.type)
        assertEquals("navigate", action?.rawType)
    }

    @Test
    fun wrongTypedOptionalValuesBecomeNullInsteadOfDroppingTheParent() {
        val dsl = decode(
            """{"type":"text","text":5,"id":"t"},
               {"type":"slider","min":"zero","max":10,"label":"l"},
               {"type":"button","label":"b","action":"not-an-object"}""",
            app = """{"name":"a","voiceEntry":"none","theme":"red","icon":true}""",
        )
        val (text, slider, button) = dsl.screens.single().components
        assertNull(text.text)
        assertEquals("t", text.id)
        assertNull(slider.min)
        assertEquals(ComponentType.SLIDER, slider.type)
        assertNull(button.action)
        assertNull(dsl.app.theme)
        assertNull(dsl.app.icon)
    }

    @Test
    fun brokenListItemsAreDropped() {
        val dsl = AppDSL.decode(
            """
            {"schemaVersion":1,"app":{"name":"a","voiceEntry":"none"},"initialScreen":"home",
             "entities":[
               {"id":"e","name":"E","retentionDays":30.0,"fields":[{"id":"f","name":"F","type":"string"},{"name":"id 無し"},"x"]},
               {"name":"id 無し","fields":[]},
               {"id":"e2","name":"E2","retentionDays":"30","fields":"not-an-array"}
             ],
             "screens":[{"id":"home","title":"h","components":[]}]}
            """.trimIndent(),
        )
        assertEquals(listOf("e", "e2"), dsl.entities.map { it.id })
        val (e, e2) = dsl.entities
        assertEquals(30, e.retentionDays)
        assertEquals(listOf("f"), e.fields.map { it.id })
        assertNull(e2.retentionDays)
        assertEquals(emptyList<Field>(), e2.fields)
    }

    @Test
    fun exprDecodesEveryShapeAndKeepsUnknownOnes() {
        val dsl = AppDSL.decode(
            """
            {"schemaVersion":1,"app":{"name":"a","voiceEntry":"none"},"initialScreen":"home",
             "entities":[{"id":"e","name":"E","fields":[
               {"id":"a","name":"A","type":"number","computed":{"op":"add","args":[{"ref":"x"},2,"s",true,{"future":1}]}},
               {"id":"b","name":"B","type":"number","computed":{"if":{"ref":"c"},"then":1,"else":{"agg":"count","entity":"e"}}},
               {"id":"c","name":"C","type":"number","computed":{"op":"add","args":"broken"}}
             ]}],
             "screens":[{"id":"home","title":"h","components":[]}]}
            """.trimIndent(),
        )
        val (a, b, c) = dsl.entities.single().fields
        val op = a.computed as ExprOp
        assertEquals(ExprOpOp.ADD, op.op)
        assertEquals(
            listOf(ExprRef("x"), ExprNumber(2.0), ExprString("s"), ExprBoolean(true), ExprUnknown(JsonObject(mapOf("future" to JsonPrimitive(1))))),
            op.args,
        )
        val cond = b.computed as ExprIf
        assertEquals(ExprRef("c"), cond.`if`)
        assertEquals(Agg.COUNT, (cond.`else` as ExprAgg).agg)
        assertEquals(emptyList<Expr>(), (c.computed as ExprOp).args)
    }

    @Test
    fun containerKeepsChildrenAndDropsBrokenOnes() {
        val dsl = decode("""{"type":"card","title":"c","children":[{"type":"text","text":"a"},7,{"type":"newThing"}]}""")
        val card = dsl.screens.single().components.single()
        assertTrue(card.type.isContainer)
        assertEquals(listOf(ComponentType.TEXT, ComponentType.UNKNOWN), card.children.map { it.type })
        assertEquals(listOf("text", "newThing"), card.children.map { it.rawType })
        assertEquals("card", card.rawType)
    }

    // ---- helpers -----------------------------------------------------------

    private fun decode(components: String, app: String = """{"name":"a","voiceEntry":"none"}"""): AppDSL = AppDSL.decode(
        """{"schemaVersion":1,"app":$app,"initialScreen":"home","screens":[{"id":"home","title":"h","components":[$components]}]}""",
    )

    private fun allComponents(list: JsonArray): List<JsonObject> = list.mapNotNull { it as? JsonObject }
        .flatMap { listOf(it) + allComponents(it["children"] as? JsonArray ?: JsonArray(emptyList())) }

    /** 最初に違う場所(パスと両方の値)。同じなら null。 */
    private fun firstDiff(path: String, a: JsonElement?, b: JsonElement?): String? = when {
        a is JsonObject && b is JsonObject -> (a.keys + b.keys).sorted().firstNotNullOfOrNull { firstDiff("$path.$it", a[it], b[it]) }
        a is JsonArray && b is JsonArray && a.size == b.size -> a.indices.firstNotNullOfOrNull { firstDiff("$path[$it]", a[it], b[it]) }
        a == b -> null
        else -> "$path: $a -> $b"
    }

    /** 比べる前にそろえる: 空の配列と null は「無い」と同じ・数は Double で比べる。 */
    private fun normalize(element: JsonElement): JsonElement = when (element) {
        is JsonObject -> JsonObject(
            element.filterValues { it !is JsonNull && !(it is JsonArray && it.isEmpty()) }.mapValues { normalize(it.value) }.toSortedMap(),
        )
        is JsonArray -> JsonArray(element.map(::normalize))
        is JsonPrimitive -> if (!element.isString && element.doubleOrNull != null) JsonPrimitive(element.doubleOrNull) else element
    }
}
