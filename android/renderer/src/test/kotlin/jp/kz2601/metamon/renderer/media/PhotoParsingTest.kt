package jp.kz2601.metamon.renderer.media

import jp.kz2601.metamon.dsl.PhotoAnalyze
import jp.kz2601.metamon.dsl.PhotoAnalyzeKind
import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.Field
import jp.kz2601.metamon.dsl.records.FieldType
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.renderer.components.media.OcrPiece
import jp.kz2601.metamon.renderer.components.media.PhotoClassifier
import jp.kz2601.metamon.renderer.components.media.PhotoClues
import jp.kz2601.metamon.renderer.components.media.PhotoColor
import jp.kz2601.metamon.renderer.components.media.PhotoReading
import jp.kz2601.metamon.renderer.components.media.ReadDate
import jp.kz2601.metamon.renderer.components.media.ReceiptParser
import jp.kz2601.metamon.renderer.components.media.needsLabels
import jp.kz2601.metamon.renderer.components.media.needsText
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Test
import java.time.ZoneId

/** #1119 photoField.analyze の純粋な部分。入出力は iOS の PhotoReadingTests / PhotoAnalyzeTests と同じ。 */
class PhotoParsingTest {
    // レシート

    @Test
    fun amountPrefersTotalLine() {
        val lines = listOf(
            "スーパーまるや", "2026年10月28日(火) 18:42", "牛乳 ¥198", "食パン ¥258", "小計 ¥456", "合計点数 2点",
            "合計 ¥493", "お預り ¥1,000", "お釣り ¥507",
        )
        assertEquals(493.0, ReceiptParser.amount(lines))
    }

    @Test
    fun amountOnNextLineAndFullWidth() {
        assertEquals(1280.0, ReceiptParser.amount(listOf("お会計", "￥１，２８０")))
        assertEquals("円記号を \\ と読んだ OCR", 3456.0, ReceiptParser.amount(listOf("TOTAL", "\\3,456")))
    }

    @Test
    fun amountFallsBackToLargestMarkedValue() {
        assertEquals(520.0, ReceiptParser.amount(listOf("コーヒー 450円", "ケーキ 520円", "TEL 076-123-4567")))
        assertNull(ReceiptParser.amount(listOf("会員番号 12345678", "ありがとうございました")))
    }

    @Test
    fun dateFormats() {
        assertEquals(ReadDate(2026, 10, 28, 18, 42), ReceiptParser.date(listOf("領収書", "2026年10月28日(火) 18:42")))
        assertEquals(ReadDate(2026, 1, 5), ReceiptParser.date(listOf("2026/1/5")))
        assertEquals(ReadDate(2026, 10, 28), ReceiptParser.date(listOf("令和8年10月28日")))
        assertNull("無い日付は拾わない", ReceiptParser.date(listOf("2026/2/30")))
        assertNull(ReceiptParser.date(listOf("合計 ¥493")))
    }

    // 保存形式

    @Test
    fun jsonValueForFieldTypes() {
        val tokyo = ZoneId.of("Asia/Tokyo")
        assertEquals(JSONValue.Num(493.0), PhotoReading.jsonValue(492.6, FieldType.CURRENCY))
        assertEquals(JSONValue.Num(87.3), PhotoReading.jsonValue(87.25, FieldType.NUMBER))
        assertEquals(JSONValue.Str("2026-10-28"), PhotoReading.jsonValue(ReadDate(2026, 10, 28), FieldType.DATE))
        assertEquals(
            JSONValue.Str("2026-10-28T18:42:00+09:00"),
            PhotoReading.jsonValue(ReadDate(2026, 10, 28, 18, 42), FieldType.DATETIME, tokyo),
        )
        assertEquals(JSONValue.Str("牛乳"), PhotoReading.jsonValue("  牛乳  ", FieldType.TEXT))
        assertNull(PhotoReading.jsonValue(" ", FieldType.STRING))
        assertNull("型が合わない組み合わせは入れない", PhotoReading.jsonValue("x", FieldType.CURRENCY))
        assertNull(PhotoReading.jsonValue(ReadDate(2026, 10, 28), FieldType.TEXT))
    }

    // 分類・ラベル

    @Test
    fun classifierUsesLabelsAndText() {
        val options = listOf("食費", "日用品", "交通", "その他")
        assertEquals("食費", PhotoClassifier.classify(options, listOf("food", "fruit"), ""))
        assertEquals("交通", PhotoClassifier.classify(options, listOf("document"), "JR 東日本 交通系IC"))
        assertNull("当てはまらなければ入れない", PhotoClassifier.classify(options, listOf("structure"), ""))
        assertEquals("多肉植物", PhotoClassifier.classify(listOf("花", "観葉植物", "多肉植物"), listOf("succulent", "plant"), ""))
    }

    @Test
    fun namesSkipGenericLabelsAndDedupe() {
        assertEquals(listOf("ラーメン", "麺"), PhotoClassifier.names(listOf("food", "ramen", "noodles", "dish"), 5))
        assertEquals(listOf("花", "ひまわり"), PhotoClassifier.names(listOf("plant", "flower", "sunflower", "rose"), 2))
        assertEquals("具体的なラベルが無ければ大まかな語", listOf("人"), PhotoClassifier.names(listOf("people", "adult"), 1))
        assertEquals("同じ日本語は 1 つ", listOf("人"), PhotoClassifier.names(listOf("person", "people"), 5))
        assertEquals(emptyList<String>(), PhotoClassifier.names(emptyList(), 5))
        assertEquals("知らないラベルは英語のまま", "hot air balloon", PhotoClassifier.japanese("hot_air_balloon"))
    }

    @Test
    fun mlKitLabelsMapToTheTableKeys() {
        assertEquals("ice_cream", PhotoClassifier.key(" Ice cream "))
        assertEquals("アイスクリーム", PhotoClassifier.japanese(PhotoClassifier.key("Ice cream")))
        assertEquals("犬", PhotoClassifier.japanese(PhotoClassifier.key("Dog")))
    }

    // 色

    @Test
    fun colorNames() {
        val cases = listOf(
            Triple(1.0, 0.0, 0.0) to "赤", Triple(1.0, 0.5, 0.0) to "オレンジ", Triple(1.0, 1.0, 0.0) to "黄色",
            Triple(0.0, 0.8, 0.0) to "緑", Triple(0.0, 1.0, 1.0) to "水色", Triple(0.0, 0.0, 1.0) to "青",
            Triple(0.5, 0.0, 1.0) to "紫", Triple(1.0, 0.4, 0.8) to "ピンク", Triple(1.0, 0.7, 0.75) to "ピンク",
            Triple(0.5, 0.3, 0.1) to "茶色", Triple(1.0, 1.0, 1.0) to "白", Triple(0.5, 0.5, 0.5) to "灰色",
            Triple(0.05, 0.05, 0.1) to "黒",
        )
        for ((rgb, want) in cases) assertEquals("$rgb", want, PhotoColor.name(rgb.first, rgb.second, rgb.third))
        assertEquals("赤", PhotoColor.dominant(listOf(0xFF0000, 0x0000FF, 0xFF0000)))
        assertEquals("alpha は見ない", "白", PhotoColor.name(0xFFFFFFFF.toInt()))
        assertEquals("同数は名前順", "灰色", PhotoColor.dominant(listOf(0x808080, 0xFFFFFF)))
        assertNull(PhotoColor.dominant(emptyList()))
    }

    // OCR の行

    @Test
    fun piecesOnTheSameRowAreJoinedLeftToRight() {
        // iOS の同じ断片を左上原点に直したもの(top = 1 - y - height)
        val pieces = listOf(
            OcrPiece("¥493", 0.6f, 0.65f, 0.2f, 0.05f),
            OcrPiece("合計", 0.1f, 0.64f, 0.2f, 0.05f),
            OcrPiece("店名", 0.1f, 0.05f, 0.3f, 0.05f),
            OcrPiece("牛乳", 0.1f, 0.45f, 0.2f, 0.05f),
        )
        assertEquals(listOf("店名", "牛乳", "合計 ¥493"), OcrPiece.lines(pieces))
    }

    // 手がかり → field

    private fun field(id: String, type: FieldType, options: List<String>? = null) = Field(id = id, name = id, type = type, options = options)

    private val more = Entity(
        id = "item", name = "記録",
        fields = listOf(
            field("photo", FieldType.PHOTO), field("name", FieldType.STRING), field("things", FieldType.TEXT),
            field("code", FieldType.STRING), field("people", FieldType.INTEGER), field("color", FieldType.STRING),
        ),
    )
    private val moreSpecs = listOf(
        PhotoAnalyze(PhotoAnalyzeKind.LABEL, "name"), PhotoAnalyze(PhotoAnalyzeKind.LABELS, "things"),
        PhotoAnalyze(PhotoAnalyzeKind.BARCODE, "code"), PhotoAnalyze(PhotoAnalyzeKind.PEOPLE_COUNT, "people"),
        PhotoAnalyze(PhotoAnalyzeKind.COLOR, "color"),
    )

    @Test
    fun moreKindsFromInjectedClues() {
        val clues = PhotoClues(
            labels = listOf("food", "sushi", "dish", "rice", "sashimi_platter", "plate", "table"),
            barcodes = listOf("9784000000000", "https://example.com"), peopleCount = 3, color = "赤",
        )
        val out = PhotoReading.apply(clues, moreSpecs, more)
        assertEquals("大まかな food を飛ばして具体的なラベル", JSONValue.Str("寿司"), out.values["name"])
        assertEquals(JSONValue.Str("寿司、ご飯、sashimi platter、plate、テーブル"), out.values["things"])
        assertEquals(JSONValue.Str("9784000000000"), out.values["code"])
        assertEquals(JSONValue.Num(3.0), out.values["people"])
        assertEquals(JSONValue.Str("赤"), out.values["color"])
        assertEquals(emptyList<String>(), out.missed)
    }

    @Test
    fun moreKindsWithNothingFound() {
        val out = PhotoReading.apply(PhotoClues(peopleCount = 0), moreSpecs, more)
        assertEquals(mapOf("people" to JSONValue.Num(0.0)), out.values)
        assertEquals(listOf("name", "things", "code", "color"), out.missed)
    }

    @Test
    fun labelFallsBackToGenericLabel() {
        val out = PhotoReading.apply(PhotoClues(labels = listOf("food", "plant")), listOf(PhotoAnalyze(PhotoAnalyzeKind.LABEL, "name")), more)
        assertEquals(JSONValue.Str("食べ物"), out.values["name"])
    }

    /** iOS の testReadsRenderedReceipt と同じレシートの行(OCR の後)から、金額・日付・分類・説明・文字を入れる。 */
    @Test
    fun receiptLinesFillAmountDateCategoryAndMemo() {
        val expense = Entity(
            id = "expense", name = "支出",
            fields = listOf(
                field("amount", FieldType.CURRENCY), field("date", FieldType.DATE), field("at", FieldType.DATETIME),
                field("category", FieldType.ENUM, listOf("食費", "日用品", "交通", "その他")),
                field("memo", FieldType.TEXT), field("about", FieldType.TEXT), field("angle", FieldType.NUMBER),
            ),
        )
        val specs = listOf(
            PhotoAnalyze(PhotoAnalyzeKind.AMOUNT, "amount"), PhotoAnalyze(PhotoAnalyzeKind.DATE, "date"),
            PhotoAnalyze(PhotoAnalyzeKind.DATE, "at"), PhotoAnalyze(PhotoAnalyzeKind.CLASSIFY, "category"),
            PhotoAnalyze(PhotoAnalyzeKind.TEXT, "memo"), PhotoAnalyze(PhotoAnalyzeKind.DESCRIBE, "about"),
            PhotoAnalyze(PhotoAnalyzeKind.JOINT_ANGLE, "angle"),
        )
        val clues = PhotoClues(lines = listOf("TAILOR MART", "2026/10/28 18:42", "Milk ¥198", "Bread ¥258", "TOTAL ¥493"), labels = listOf("receipt", "food"))
        val out = PhotoReading.apply(clues, specs, expense, ZoneId.of("Asia/Tokyo"))
        assertEquals(JSONValue.Num(493.0), out.values["amount"])
        assertEquals(JSONValue.Str("2026-10-28"), out.values["date"])
        assertEquals(JSONValue.Str("2026-10-28T18:42:00+09:00"), out.values["at"])
        assertEquals("options が無ければ into の enum の選択肢", JSONValue.Str("食費"), out.values["category"])
        assertEquals(JSONValue.Str("TAILOR MART\n2026/10/28 18:42\nMilk ¥198\nBread ¥258\nTOTAL ¥493"), out.values["memo"])
        assertEquals(JSONValue.Str("写っているもの: レシート、食べ物"), out.values["about"])
        assertEquals("骨格は読まない", listOf("angle"), out.missed)
    }

    @Test
    fun blankPhotoMissesEverything() {
        val expense = Entity(id = "e", name = "e", fields = listOf(field("amount", FieldType.CURRENCY), field("date", FieldType.DATE)))
        val specs = listOf(PhotoAnalyze(PhotoAnalyzeKind.AMOUNT, "amount"), PhotoAnalyze(PhotoAnalyzeKind.DATE, "date"))
        val out = PhotoReading.apply(PhotoClues(), specs, expense)
        assertEquals(emptyMap<String, JSONValue>(), out.values)
        assertEquals(listOf("amount", "date"), out.missed)
    }

    @Test
    fun onlyRequiredReadersRun() {
        val specs = listOf("label", "labels", "barcode", "peopleCount", "color").map { kind ->
            PhotoAnalyze(PhotoAnalyzeKind.entries.first { it.wire == kind }, "x")
        }
        assertEquals(listOf(true, true, false, false, false), specs.map { it.needsLabels })
        assertFalse("OCR は回さない", specs.any { it.needsText })
    }
}
