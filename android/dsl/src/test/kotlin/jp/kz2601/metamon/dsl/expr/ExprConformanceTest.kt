package jp.kz2601.metamon.dsl.expr

import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.FieldCoding
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.Record
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import org.junit.After
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Test
import java.io.File
import java.time.Instant
import java.time.ZoneId
import java.time.format.DateTimeFormatter

/**
 * 計算式の共有テストデータ `docs/dsl/conformance/expr` の JSON(#545)を Android の評価器で回す。
 * `expected` は iOS の評価結果(iOS が正)。全ケースが一致すること。
 */
class ExprConformanceTest {
    private val savedZone = FieldCoding.zone

    @After
    fun restore() {
        FieldCoding.zone = savedZone
    }

    private val dir: File = File(
        requireNotNull(System.getProperty("metamon.dslExamplesDir")) { "metamon.dslExamplesDir が未設定(Gradle から実行する)" },
    ).resolve("../conformance/expr").normalize()

    @Test
    fun conformance() {
        val files = dir.listFiles { f -> f.extension == "json" }.orEmpty().sortedBy { it.name }
        assertTrue("ケースのファイルが無い: $dir", files.isNotEmpty())
        val failures = mutableListOf<String>()
        var total = 0
        for (file in files) {
            val root = Json.parseToJsonElement(file.readText()) as JsonObject
            val defaults = root["defaults"] as JsonObject
            for (c in root["cases"] as JsonArray) {
                c as JsonObject
                val name = (c["name"] as JsonPrimitive).content
                total++
                val tzName = str(c, "timeZone") ?: str(defaults, "timeZone")!!
                val got = try {
                    encode(evaluate(c, defaults, ZoneId.of(tzName)), ZoneId.of(tzName))
                } catch (e: Exception) {
                    failures += "${file.name} $name: 例外 $e"
                    continue
                }
                val want = c["expected"]?.let { JSONValue.from(it) }
                if (want == null) {
                    failures += "${file.name} $name: expected が無い"
                } else if (got != want) {
                    failures += "${file.name} $name: got ${got.toJsonElement()} want ${want.toJsonElement()}"
                }
            }
        }
        assertTrue(total > 0)
        if (failures.isNotEmpty()) fail("${failures.size}/$total 件が iOS と違う:\n" + failures.joinToString("\n"))
        println("expr conformance: $total cases OK")
    }

    private fun str(o: JsonObject, key: String) = (o[key] as? JsonPrimitive)?.content

    private fun evaluate(c: JsonObject, defaults: JsonObject, zone: ZoneId): ExprValue? {
        val now = FieldCoding.strictDatetime(str(c, "now") ?: str(defaults, "now")!!)!!
        // 評価器の外の "YYYY-MM-DD" の読み書きも端末のタイムゾーンを使うので、ケースのタイムゾーンに合わせる
        FieldCoding.zone = zone
        val entities = ((c["entities"] ?: defaults["entities"]) as? JsonArray)?.map { Entity.from(it) } ?: emptyList()
        val records = ((c["records"] ?: defaults["records"]) as? JsonObject)
            ?.mapValues { (_, rows) -> (rows as JsonArray).map { Record.from(it) } } ?: emptyMap()
        val ev = ExprEvaluator(entities, { records[it] ?: emptyList() }, now, zone)
        val row = (c["row"] as? JsonObject)?.let { r ->
            val id = str(r, "entity")
            ExprEvaluator.Row(entities.firstOrNull { it.id == id }, Record.from(r["record"]!!))
        }
        return ev.evaluate(Expr.from(c["expr"]!!), row)
    }

    /** 評価結果 → `{"type", "value", "text"}`(null は `{"type":"nil"}`)。日付はケースのタイムゾーンの RFC 3339。 */
    private fun encode(v: ExprValue?, zone: ZoneId): JSONValue {
        if (v == null) return JSONValue.of(mapOf("type" to "nil"))
        val text = v.text
        return when (v) {
            is ExprValue.Number -> {
                val n = v.value
                val value: Any = if (n.isNaN()) "nan" else if (n.isInfinite()) (if (n > 0) "inf" else "-inf") else n
                JSONValue.of(mapOf("type" to "number", "value" to value, "text" to text))
            }
            is ExprValue.Str -> JSONValue.of(mapOf("type" to "string", "value" to v.value, "text" to text))
            is ExprValue.Bool -> JSONValue.of(mapOf("type" to "bool", "value" to v.value, "text" to text))
            is ExprValue.Date -> {
                val f = DateTimeFormatter.ofPattern("uuuu-MM-dd'T'HH:mm:ssXXX").withZone(zone)
                JSONValue.of(mapOf("type" to "date", "value" to f.format(Instant.ofEpochSecond(Math.floorDiv(v.value.epochSecond, 1L))), "text" to text))
            }
        }
    }
}
