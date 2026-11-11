package jp.kz2601.metamon.dsl.ai

import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.DslJson
import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.Field
import jp.kz2601.metamon.dsl.records.FieldCoding
import jp.kz2601.metamon.dsl.records.Fixtures
import jp.kz2601.metamon.dsl.records.JSONValue
import kotlinx.serialization.json.Json
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import java.time.Instant
import java.time.ZoneId

/** aiButton `outputs`: fields の項目を型に直してフォームに入れる(iOS の AIOutputsTests・backend `ai_outputs.go`)。 */
class AiOutputsTest {
    private val now = Instant.parse("2026-11-08T03:00:00Z")
    private var savedZone: ZoneId = FieldCoding.zone

    @Before
    fun setUp() {
        savedZone = FieldCoding.zone
        FieldCoding.zone = Fixtures.TOKYO
    }

    @After
    fun tearDown() {
        FieldCoding.zone = savedZone
    }

    private val entity: Entity = Entity.from(
        Json.parseToJsonElement(
            """
            {"id":"ev","name":"予定","fields":[
             {"id":"title","name":"題名","type":"string"},
             {"id":"start","name":"日時","type":"datetime"},
             {"id":"day","name":"日","type":"date"},
             {"id":"score","name":"点数","type":"integer"},
             {"id":"rate","name":"率","type":"number"},
             {"id":"kind","name":"種類","type":"enum","options":["講演","展示","Live"]},
             {"id":"done","name":"済","type":"boolean"}]}
            """.trimIndent(),
        ),
    )

    private fun field(id: String): Field = entity.field(id)!!

    private fun num(n: Double) = JSONValue.Num(n)

    private fun str(s: String) = JSONValue.Str(s)

    @Test
    fun componentDecodesOutputs() {
        val c = DslJson.decodeFromString(
            Component.serializer(),
            """{"type":"aiButton","task":"generate","prompt":"x","shape":"fields","items":["題名"],"outputs":{"題名":"title"}}""",
        )
        assertEquals(mapOf("題名" to "title"), AiOutputs.from(c.outputs))
        val broken = DslJson.decodeFromString(Component.serializer(), """{"type":"aiButton","task":"generate","prompt":"x","outputs":["title"]}""")
        assertNull("読めない outputs は無視(表示だけ)", AiOutputs.from(broken.outputs))
    }

    @Test
    fun numbers() {
        val score = field("score")
        val rate = field("rate")
        assertEquals(num(85.0), AiOutputs.value("85", score))
        assertEquals(num(85.0), AiOutputs.value("85 点", score))
        assertEquals("integer は丸める", num(86.0), AiOutputs.value("８５．６", score))
        assertEquals(num(3000.0), AiOutputs.value("約 3,000 円", rate))
        assertEquals(num(0.75), AiOutputs.value("0.75", rate))
        assertNull("数が 2 つは読まない", AiOutputs.value("85/100", score))
        assertNull(AiOutputs.value("よくできました", score))
    }

    @Test
    fun kanjiMultipliers() {
        val score = field("score")
        val rate = field("rate")
        assertEquals("「3万円」を 3 にしない", num(30000.0), AiOutputs.value("3万円", rate))
        assertEquals(num(15000.0), AiOutputs.value("約 1.5万人", rate))
        assertEquals(num(35000.0), AiOutputs.value("３万５０００円", rate))
        assertEquals(num(120_000_000.0), AiOutputs.value("1億2000万", rate))
        assertEquals(num(30_000_000.0), AiOutputs.value("3千万円", rate))
        assertEquals(num(5000.0), AiOutputs.value("5千", score))
        assertNull("範囲は読まない", AiOutputs.value("3万〜5万円", rate))
        assertNull("残りが曖昧なら空", AiOutputs.value("3万円以上", rate))
        assertNull(AiOutputs.value("千円札", rate))
        val thousand = rate.copy(unit = "千円")
        assertEquals("単位が千円なら単位どおり", num(3.0), AiOutputs.value("3千円", thousand))
    }

    @Test
    fun yearlessDatesRollToNextYear() {
        val day = field("day")
        val start = field("start")
        val dec = Instant.parse("2026-12-20T03:00:00Z")
        assertEquals("12 月の「1/10」は来年", str("2027-01-10"), AiOutputs.value("1/10", day, dec))
        assertEquals(str("2027-01-10"), AiOutputs.value("1月10日", day, dec))
        assertEquals("少し前は今年のまま", str("2026-12-01"), AiOutputs.value("12/1", day, dec))
        assertEquals("年があればそのまま", str("2026-01-10"), AiOutputs.value("2026/1/10", day, dec))
        val jan = FieldCoding.date(AiOutputs.value("1/10 18:00", start, dec)!!)!!
        assertEquals(2027, jan.atZone(FieldCoding.zone).year)
    }

    @Test
    fun dateFieldKeepsGivenOffsetDay() {
        val day = field("day")
        assertEquals("TZ で日をずらさない", str("2026-11-20"), AiOutputs.value("2026-11-20T23:30:00-05:00", day, now))
        assertEquals(str("2026-11-20"), AiOutputs.value("2026-11-20T00:30:00+09:00", day, now))
        assertNull(AiOutputs.value("2026-02-30T10:00:00Z", day, now))
    }

    @Test
    fun applyDoesNotOverwrite() {
        val fills = AiOutputs.fills(
            listOf(AiTextField("題名", "秋の音楽祭"), AiTextField("点数", "85"), AiTextField("種類", "展示"), AiTextField("日時", "不明")),
            mapOf("題名" to "title", "点数" to "score", "種類" to "kind", "日時" to "start"),
            entity,
            now,
        )
        val values = mutableMapOf<String, JSONValue>(
            "title" to str("自分で書いた題"), "score" to JSONValue.Null, "kind" to str("講演"), "start" to str("2026-11-20T09:00:00Z"),
        )
        val r = AiOutputs.apply(fills, values) { id, _ -> id == "kind" }
        assertEquals(listOf("score", "kind"), r.applied.map { it.field })
        assertEquals(listOf("題名"), r.kept.map { it.fieldName })
        assertEquals("入力済みは上書きしない", str("自分で書いた題"), values["title"])
        assertEquals(num(85.0), values["score"])
        assertEquals("入れ替えてよい値(初期値・前回の AI)は入れる", str("展示"), values["kind"])
        assertEquals("読めない値は触らない", str("2026-11-20T09:00:00Z"), values["start"])
        val same = mutableMapOf<String, JSONValue>("title" to str("秋の音楽祭"))
        assertEquals("同じ値は入れなかった扱いにしない", emptyList<AiOutputs.Fill>(), AiOutputs.apply(fills.take(1), same) { _, _ -> false }.kept)
    }

    @Test
    fun dates() {
        val start = field("start")
        val day = field("day")
        val a = AiOutputs.value("2026/11/20 18:00", start, now)
        assertEquals(AiOutputs.value("2026-11-20T18:00", start, now), a)
        assertNotNull(a)
        assertEquals("年なしは今年・範囲は始まり", a, AiOutputs.value("11/20(金) 18:00〜20:00", start, now))
        assertEquals(a, AiOutputs.value("2026年11月20日(金)18時", start, now))
        assertEquals(str("2026-11-20"), AiOutputs.value("11月20日", day, now))
        assertNull(AiOutputs.value("来週のどこか", day, now))
        assertNull(AiOutputs.value("不明", day, now))
    }

    @Test
    fun enumAndText() {
        val kind = field("kind")
        val title = field("title")
        assertEquals(str("展示"), AiOutputs.value("展示", kind))
        assertEquals(str("Live"), AiOutputs.value("live", kind))
        assertNull("options に無ければ空", AiOutputs.value("ワークショップ", kind))
        assertEquals(str("秋の音楽祭"), AiOutputs.value("  秋の音楽祭 \n", title))
        assertNull("分からない答えは空", AiOutputs.value("-", title))
        assertNull("boolean は入れない", AiOutputs.value("yes", field("done")))
    }

    @Test
    fun fillsFollowResultOrderAndSkipUnmapped() {
        val fields = listOf(AiTextField("題名", "秋の音楽祭"), AiTextField("メモ", "x"), AiTextField("日時", "未定"), AiTextField("種類", "展示"))
        val fills = AiOutputs.fills(fields, mapOf("題名" to "title", "日時" to "start", "種類" to "kind", "済" to "done"), entity, now)
        assertEquals(listOf("title", "start", "kind"), fills.map { it.field })
        assertEquals(listOf("題名", "日時", "種類"), fills.map { it.fieldName })
        assertEquals(str("秋の音楽祭"), fills[0].value)
        assertNull("読めない値は入れない", fills[1].value)
        assertEquals(str("展示"), fills[2].value)
    }

    @Test
    fun formatsAndHint() {
        assertNull(AiOutputs.formats(listOf("題名"), mapOf("題名" to "title"), entity))
        val f = AiOutputs.formats(
            listOf("点数", "日", "種類", "題名"),
            mapOf("点数" to "score", "日" to "day", "種類" to "kind", "題名" to "title"),
            entity,
        )!!
        assertEquals(
            listOf(AiTextFormat("点数", "number"), AiTextFormat("日", "date"), AiTextFormat("種類", "enum", listOf("講演", "展示", "Live"))),
            f,
        )
        assertEquals("Format: 点数=number only / 日=YYYY/M/D (M/D if no year) (unknown: -)", AiOutputs.hint(f.take(2), english = true))
        assertEquals("書き方: 種類=講演/展示/Live のどれか(分からなければ -)", AiOutputs.hint(listOf(f[2]), english = false))
        assertNull(AiOutputs.hint(null, english = false))
    }

    /** formats は送る body に載る(無ければキーごと送らない)。 */
    @Test
    fun formatsAreEncodedOnlyWhenPresent() {
        val json = Json { explicitNulls = false; encodeDefaults = true }
        val with = json.encodeToString(
            AiTextRequest.serializer(),
            AiTextRequest(instruction = "x", shape = AiTextShape.FIELDS, items = listOf("点数"), formats = listOf(AiTextFormat("点数", "number"))),
        )
        assertTrue(with, with.contains(""""formats":[{"item":"点数","kind":"number"}]"""))
        assertFalse(json.encodeToString(AiTextRequest.serializer(), AiTextRequest(instruction = "x", shape = AiTextShape.LINE)).contains("formats"))
    }
}
