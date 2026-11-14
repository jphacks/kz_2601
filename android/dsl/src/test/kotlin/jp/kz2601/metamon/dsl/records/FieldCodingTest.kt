package jp.kz2601.metamon.dsl.records

import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.util.Locale

/** iOS の FieldCodingTests を写したもの。 */
class FieldCodingTest {
    private val savedZone = FieldCoding.zone

    init {
        FieldCoding.zone = Fixtures.TOKYO
    }

    @After
    fun restore() {
        FieldCoding.zone = savedZone
    }

    private fun field(type: String) = Field.from(kotlinx.serialization.json.Json.parseToJsonElement("""{"id":"f","name":"F","type":"$type"}"""))

    private fun currencyField(unit: String?): Field {
        val u = if (unit == null) "" else ",\"unit\":\"$unit\""
        return Field.from(kotlinx.serialization.json.Json.parseToJsonElement("{\"id\":\"f\",\"name\":\"F\",\"type\":\"currency\"$u}"))
    }

    /** #121: 未入力(null)は null を返し、payload からキーごと落ちること。 */
    @Test
    fun normalizeDropsNull() {
        for (type in listOf("photo", "location", "string", "text", "enum", "date", "datetime", "integer", "currency", "number")) {
            assertNull("type=$type", FieldCoding.normalize(JSONValue.Null, field(type)))
        }
    }

    @Test
    fun normalizeDropsEmptyString() {
        for (type in listOf("photo", "location", "string", "integer")) {
            assertNull("type=$type", FieldCoding.normalize(JSONValue.Str(""), field(type)))
        }
    }

    @Test
    fun normalizeKeepsValues() {
        assertEquals(JSONValue.number(1234), FieldCoding.normalize(JSONValue.Str("1,234"), field("currency")))
        assertEquals(JSONValue.Str("abc"), FieldCoding.normalize(JSONValue.Str("abc"), field("string")))
        assertEquals(JSONValue.Str("35.0,139.0"), FieldCoding.normalize(JSONValue.Str("35.0,139.0"), field("location")))
        assertEquals(JSONValue.Bool(false), FieldCoding.normalize(JSONValue.Null, field("boolean")))
    }

    /** #140: 一覧の日付は曜日付きで。今年なら年を省く。 */
    @Test
    fun displayDateJaAndEn() {
        val now = FieldCoding.strictDay("2026-06-01")!!
        val d = field("date")
        val ja = Locale.JAPAN
        val en = Locale.US
        assertEquals("10/24(土)", FieldCoding.display(JSONValue.Str("2026-10-24"), d, ja, now))
        assertEquals("2025/10/24(金)", FieldCoding.display(JSONValue.Str("2025-10-24"), d, ja, now))
        assertEquals("Sat, Oct 24", FieldCoding.display(JSONValue.Str("2026-10-24"), d, en, now))
        assertEquals("Fri, Oct 24, 2025", FieldCoding.display(JSONValue.Str("2025-10-24"), d, en, now))
        // 解釈できない値はそのまま
        assertEquals("来週", FieldCoding.display(JSONValue.Str("来週"), d, ja, now))
        assertEquals("", FieldCoding.display(JSONValue.Null, d, ja, now))
    }

    /** 端末の暦が和暦・仏暦でも、日本語の固定書式は西暦で出す。 */
    @Test
    fun displayDateJaIgnoresNonGregorianCalendar() {
        val now = FieldCoding.strictDay("2026-06-01")!!
        val d = field("date")
        for (tag in listOf("ja-JP-u-ca-japanese", "ja-JP-u-ca-buddhist")) {
            val locale = Locale.forLanguageTag(tag)
            assertEquals(tag, "2025/10/24(金)", FieldCoding.display(JSONValue.Str("2025-10-24"), d, locale, now))
            assertEquals(tag, "10/24(土)", FieldCoding.display(JSONValue.Str("2026-10-24"), d, locale, now))
        }
    }

    @Test
    fun displayDatetimeHasWeekdayAndTime() {
        val now = FieldCoding.strictDay("2026-06-01")!!
        val s = FieldCoding.display(JSONValue.Str("2026-10-24T12:30:00Z"), field("datetime"), Locale.JAPAN, now)
        assertTrue(s, s.startsWith("10/2"))
        assertTrue(s, s.contains(":30"))
        assertFalse(s, s.contains("2026"))
    }

    /** #555: あり得ない日時は繰り上げて読まない。正しい形(オフセット・小数秒・1 桁の月日・うるう日)は読む。 */
    @Test
    fun dateRejectsImpossibleDatetimes() {
        for (s in listOf(
            "2026-02-30T10:00:00Z", "2026-10-24T24:00:00Z", "2026-10-24T10:60:00+09:00", "2026-12-31T23:59:60Z",
            "2027-02-29T00:00:00+0900", "2026-02-30T10:00:00.5Z", "2026-02-30", "2027-02-29", "2026-2-30", "0-01-01",
        )) {
            assertNull(s, FieldCoding.date(JSONValue.Str(s)))
        }
        fun epoch(s: String) = FieldCoding.date(JSONValue.Str(s))?.let { it.epochSecond + it.nano / 1e9 }
        assertEquals(1_792_836_000.0, epoch("2026-10-24T10:00:00Z"))
        assertEquals(1_792_836_000.0, epoch("2026-10-24T19:00:00+09:00"))
        assertEquals(1_792_836_000.0, epoch("2026-10-24T05:00:00-0500"))
        assertEquals(1_792_836_000.25, epoch("2026-10-24T10:00:00.250Z"))
        assertEquals(1_835_499_599.0, epoch("2028-02-29T23:59:59-05:00"))
        assertNotNull(FieldCoding.date(JSONValue.Str("2028-02-29")))
        assertNotNull(FieldCoding.date(JSONValue.Str("2026-1-5")))
    }

    /** #150: unit が通貨そのもの(円・¥・JPY・$ 等)なら ¥ と重ねない。 */
    @Test
    fun currencySuffixDropsCurrencyUnits() {
        for (unit in listOf("円", "¥", "￥", "JPY", "jpy", "yen", "$", "USD", "dollars", " 円 ", "", null)) {
            assertNull("unit=$unit", FieldCoding.currencySuffix(unit))
        }
    }

    @Test
    fun currencySuffixKeepsRest() {
        assertEquals("/時", FieldCoding.currencySuffix("円/時"))
        assertEquals("/ 月", FieldCoding.currencySuffix("円 / 月"))
        assertEquals("per hour", FieldCoding.currencySuffix("yen per hour"))
        assertEquals("/h", FieldCoding.currencySuffix("/h"))
        assertEquals("per month", FieldCoding.currencySuffix("per month"))
    }

    /** #150: 一覧の金額は「¥800」(「¥800 円」にしない)。per 単位は後ろに付ける。 */
    @Test
    fun displayCurrencyWithUnit() {
        val ja = Locale.JAPAN
        assertEquals("¥800", FieldCoding.display(JSONValue.number(800), currencyField(null), ja))
        assertEquals("¥800", FieldCoding.display(JSONValue.number(800), currencyField("円"), ja))
        assertEquals("¥1,200", FieldCoding.display(JSONValue.number(1200), currencyField("JPY"), ja))
        assertEquals("¥15", FieldCoding.display(JSONValue.number(15), currencyField("$"), ja))
        assertEquals("¥1,200/時", FieldCoding.display(JSONValue.number(1200), currencyField("円/時"), ja))
        assertEquals("¥1,200 per hour", FieldCoding.display(JSONValue.number(1200), currencyField("yen per hour"), ja))
        assertEquals("-¥500", FieldCoding.display(JSONValue.number(-500), currencyField("円"), ja))
    }

    /** #150: boolean / photo の語は渡された words を使う。既定は日本語。 */
    @Test
    fun displayBooleanWords() {
        val b = field("boolean")
        val p = field("photo")
        assertEquals("はい", FieldCoding.display(JSONValue.Bool(true), b))
        assertEquals("いいえ", FieldCoding.display(JSONValue.Bool(false), b))
        val en = FieldCoding.Words(yes = "Yes", no = "No", photo = "Photo")
        assertEquals("Yes", FieldCoding.display(JSONValue.Bool(true), b, words = en))
        assertEquals("No", FieldCoding.display(JSONValue.Bool(false), b, words = en))
        assertEquals("Photo", FieldCoding.display(JSONValue.Str("https://x/y.jpg"), p, words = en))
        assertEquals("", FieldCoding.display(JSONValue.Null, p, words = en))
    }

    @Test
    fun plainNumberAndDefaults() {
        assertEquals("7.25", FieldCoding.plainNumber(7.25))
        assertEquals("7.3", FieldCoding.plainNumber(7.3))
        assertEquals("12", FieldCoding.plainNumber(12.0))
        val now = FieldCoding.strictDatetime("2026-10-24T03:00:00Z")!!
        assertEquals(JSONValue.Str("2026-10-24"), FieldCoding.defaultValue(field("date"), now))
        assertEquals(JSONValue.Str("2026-10-24T03:00:00Z"), FieldCoding.defaultValue(field("datetime"), now))
        assertEquals(JSONValue.Bool(false), FieldCoding.defaultValue(field("boolean"), now))
        // 選択肢から消えた既定値は使わない(#194)
        val e = Field.from(kotlinx.serialization.json.Json.parseToJsonElement("""{"id":"f","name":"F","type":"enum","options":["a","b"],"defaultValue":"z"}"""))
        assertEquals(JSONValue.Str("a"), FieldCoding.defaultValue(e, now))
    }
}
