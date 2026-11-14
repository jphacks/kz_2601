package jp.kz2601.metamon.renderer

import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentStyle
import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.dsl.decode
import jp.kz2601.metamon.dsl.expr.DSLClock
import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.FieldCoding
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.dsl.records.entityModels
import jp.kz2601.metamon.renderer.components.inputs.InputRules
import jp.kz2601.metamon.renderer.form.ColorHex
import jp.kz2601.metamon.renderer.form.DateInput
import jp.kz2601.metamon.renderer.form.FormChoices
import jp.kz2601.metamon.renderer.form.FormDraft
import jp.kz2601.metamon.renderer.form.FormPrefill
import jp.kz2601.metamon.renderer.form.NumericInput
import jp.kz2601.metamon.renderer.form.PickerRules
import jp.kz2601.metamon.renderer.form.ScreenForm
import jp.kz2601.metamon.renderer.form.TextInputRules
import jp.kz2601.metamon.renderer.form.bool
import jp.kz2601.metamon.renderer.form.color
import jp.kz2601.metamon.renderer.form.date
import jp.kz2601.metamon.renderer.form.double
import jp.kz2601.metamon.renderer.form.numberText
import jp.kz2601.metamon.renderer.form.seedTimeIfEmpty
import jp.kz2601.metamon.renderer.form.setBool
import jp.kz2601.metamon.renderer.form.setColor
import jp.kz2601.metamon.renderer.form.setDate
import jp.kz2601.metamon.renderer.form.setDouble
import jp.kz2601.metamon.renderer.form.setNumberText
import jp.kz2601.metamon.renderer.form.setText
import jp.kz2601.metamon.renderer.form.setTime
import jp.kz2601.metamon.renderer.form.toggleId
import jp.kz2601.metamon.renderer.registry.ComponentRegistry
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Before
import org.junit.Test
import java.time.Instant
import java.time.ZoneId

/** 入力部品が下書きに書く値の形(iOS の FormDraft の Binding と同じ)・必須・型の変換・画面ごとの下書きの作り方(#858)。 */
class FormInputsTest {
    private val dsl = AppDSL.decode(
        """
        {"schemaVersion":1,"app":{"name":"t","voiceEntry":"none"},"initialScreen":"form",
         "entities":[
          {"id":"item","name":"品目","titleField":"name","fields":[
            {"id":"name","name":"名前","type":"string","required":true},
            {"id":"note","name":"メモ","type":"string"},
            {"id":"count","name":"個数","type":"integer","required":true},
            {"id":"weight","name":"重さ","type":"number"},
            {"id":"price","name":"値段","type":"currency","unit":"円"},
            {"id":"done","name":"済み","type":"boolean"},
            {"id":"day","name":"日付","type":"date"},
            {"id":"at","name":"日時","type":"datetime"},
            {"id":"start","name":"開始","type":"string"},
            {"id":"color","name":"色","type":"string"},
            {"id":"kind","name":"種類","type":"enum","options":["A","B"]},
            {"id":"shop","name":"店","type":"ref","refEntity":"shop"},
            {"id":"people","name":"人","type":"refs","refEntity":"shop"},
            {"id":"secret","name":"秘密","type":"string","hidden":true},
            {"id":"total","name":"合計","type":"integer","computed":{"op":"mul","args":[{"ref":"count"},{"ref":"price"}]}}
          ]},
          {"id":"shop","name":"お店","titleField":"title","fields":[{"id":"title","name":"店名","type":"string"}]},
          {"id":"profile","name":"設定","single":true,"fields":[{"id":"nick","name":"呼び名","type":"string"}]}
         ],
         "screens":[{"id":"form","title":"品目","entity":"item","components":[{"type":"textField","field":"name"}]}]}
        """.trimIndent(),
    )
    private val entities = dsl.entityModels()
    private val item = entities.first { it.id == "item" }

    private class FakeRuntime(
        override val entityModels: List<Entity>,
        records: Map<String, List<Record>> = emptyMap(),
        private val editable: Boolean = true,
        private val uid: String? = null,
    ) : DSLRuntime {
        override val recordsFlow: StateFlow<Map<String, List<Record>>> = MutableStateFlow(records)
        override fun onVoiceInput(componentId: String?) = Unit
        override fun canEdit(entity: String, record: Record) = editable
        override fun currentUid() = uid
    }

    private val savedZone = FieldCoding.zone

    @Before
    fun setUp() {
        DSLClock.fixed = Instant.parse("2026-11-12T03:00:00Z")
        FieldCoding.zone = ZoneId.of("Asia/Tokyo")
    }

    @After
    fun tearDown() {
        DSLClock.fixed = null
        FieldCoding.zone = savedZone
    }

    @Test
    fun registryHasTheInputs() {
        val wires = InputRules.inputTypes.map { it.wire }.toSet()
        assertEquals("#858 の 12 種 + 写真・手書き・動画", 15, wires.size)
        assertTrue(ComponentRegistry.standard.registeredTypes.containsAll(wires))
    }

    @Test
    fun textAndNumbersAreStoredInTheSameShapeAsIos() {
        val f = FormDraft(item)
        f.setText("name", "りんご")
        f.setNumberText("count", "1,200")
        f.setNumberText("weight", "1.")
        assertEquals("打ちかけの小数点は残す", "1.", f.numberText("weight"))
        f.setNumberText("weight", "1.5")
        f.setNumberText("price", "300")
        f.setBool("done", true)

        assertEquals(JSONValue.Num(1200.0), f.values["count"])
        val payload = f.payload
        assertEquals(JSONValue.Str("りんご"), payload["name"])
        assertEquals(JSONValue.Num(1200.0), payload["count"])
        assertEquals(JSONValue.Num(1.5), payload["weight"])
        assertEquals(JSONValue.Num(300.0), payload["price"])
        assertEquals(JSONValue.Bool(true), payload["done"])
        assertFalse("computed は保存しない", payload.containsKey("total"))
        assertTrue(f.bool("done"))
    }

    @Test
    fun unreadableNumberIsDroppedOnSave() {
        val f = FormDraft(item)
        f.setNumberText("weight", "abc")
        assertEquals(JSONValue.Str("abc"), f.values["weight"])
        assertFalse(f.payload.containsKey("weight"))
        f.setNumberText("weight", "")
        assertEquals(JSONValue.Null, f.values["weight"])
    }

    @Test
    fun requiredIsCheckedOnTypedValues() {
        val f = FormDraft(item)
        assertEquals(listOf("名前", "個数"), f.flagMissingRequired())
        f.setText("name", "x")
        f.setNumberText("count", "abc")
        assertEquals("読めない数は空と同じ", listOf("個数"), f.flagMissingRequired())
        assertTrue(f.isFlagged("count"))
        f.setNumberText("count", "2")
        assertFalse("入れたら印は消える", f.isFlagged("count"))
        assertTrue(f.flagMissingRequired().isEmpty())
    }

    @Test
    fun slidersRoundIntegersButNotNumbers() {
        val f = FormDraft(item)
        f.setDouble("count", 2.5)
        assertEquals("Swift の rounded()(0.5 は遠い方)", JSONValue.Num(3.0), f.values["count"])
        f.setDouble("weight", 2.5)
        assertEquals(JSONValue.Num(2.5), f.values["weight"])
        assertEquals(7.0, FormDraft(item).double("count", 7.0), 0.0)
    }

    @Test
    fun datesUseDayOrRfc3339() {
        val f = FormDraft(item)
        assertEquals("date の既定は今日", JSONValue.Str("2026-11-12"), f.values["day"])
        assertEquals(JSONValue.Str("2026-11-12T03:00:00Z"), f.values["at"])
        f.setDate("day", Instant.parse("2026-12-31T20:00:00Z"))
        assertEquals("端末のタイムゾーンの暦日", JSONValue.Str("2027-01-01"), f.values["day"])
        f.setDate("at", Instant.parse("2026-12-31T20:00:00Z"))
        assertEquals(JSONValue.Str("2026-12-31T20:00:00Z"), f.values["at"])

        // DatePicker(UTC の 0 時)との受け渡しで日がずれない
        val picked = DateInput.pickerMillis(f.date("day"))
        assertEquals(Instant.parse("2027-01-01T00:00:00Z").toEpochMilli(), picked)
        f.setDate("day", DateInput.fromPicker(picked, f.date("day")))
        assertEquals(JSONValue.Str("2027-01-01"), f.values["day"])
    }

    @Test
    fun timePickerStoresHourMinuteOrDatetime() {
        val f = FormDraft(item)
        f.seedTimeIfEmpty("start")
        assertEquals("見えている今の時刻を保存値にする(#229)", JSONValue.Str("12:00"), f.values["start"])
        f.setTime("start", 7, 5)
        assertEquals(JSONValue.Str("07:05"), f.values["start"])
        f.seedTimeIfEmpty("start")
        assertEquals("入力済みなら変えない", JSONValue.Str("07:05"), f.values["start"])
        f.setTime("at", 9, 30)
        assertEquals("datetime はその日の時刻(RFC 3339)", JSONValue.Str("2026-11-12T00:30:00Z"), f.values["at"])
    }

    @Test
    fun colorIsHashRrggbb() {
        val f = FormDraft(item)
        assertNull(f.color("color"))
        f.setColor("color", 0x34C759)
        assertEquals(JSONValue.Str("#34C759"), f.values["color"])
        assertEquals(0x34C759, f.color("color"))
        assertNull(ColorHex.parse("#12"))
    }

    @Test
    fun refsKeepTargetOrderAndUnknownIds() {
        val f = FormDraft(item)
        f.values["people"] = JSONValue.Arr(listOf(JSONValue.Str("gone")))
        f.toggleId("people", "s2", listOf("s1", "s2", "s3"))
        f.toggleId("people", "s1", listOf("s1", "s2", "s3"))
        assertEquals(JSONValue.Arr(listOf("s1", "s2", "gone").map { JSONValue.Str(it) }), f.values["people"])
        f.toggleId("people", "s2", listOf("s1", "s2", "s3"))
        assertEquals(JSONValue.Arr(listOf("s1", "gone").map { JSONValue.Str(it) }), f.payload["people"])
    }

    @Test
    fun refOptionsUseTargetTitles() {
        val rt = FakeRuntime(entities, mapOf("shop" to listOf(Record("s1", mapOf("title" to JSONValue.Str("八百屋"))))))
        val shop = item.field("shop")!!
        assertEquals(listOf("s1" to "八百屋"), FormChoices.refOptions(shop, rt).map { it.id to it.title })
        assertEquals("お店", FormChoices.refEntityName(shop, rt, "linked item"))
        val f = FormDraft(item)
        f.setNumberText("count", "2")
        f.setNumberText("price", "150")
        assertEquals("computed は下書きで計算する", "300", FormChoices.computedText(item.field("total")!!, f, rt))
    }

    @Test
    fun overrideByFieldType() {
        val f = FormDraft(item)
        val picker = Component(type = ComponentType.TEXT_FIELD, field = "shop")
        assertEquals("ref は部品の種類によらず選択に", "shop", InputRules.override(picker, f)?.id)
        assertNull(InputRules.override(Component(type = ComponentType.COLOR_PICKER, field = "shop"), f))
        assertEquals("needsForm", InputRules.state(Component(type = ComponentType.TEXT_FIELD, field = "name"), null, null))
        assertEquals("needsForm", InputRules.state(Component(type = ComponentType.TEXT_FIELD), f, null))
    }

    @Test
    fun hiddenFieldsOnlyForTheAuthor() {
        val c = Component(type = ComponentType.TEXT_FIELD, field = "secret")
        assertFalse("新規は出す", FormDraft(item).hides(c, null))
        val others = FormDraft(item, Record("r1", author = "u1"))
        assertTrue(others.hides(c, "u2"))
        assertTrue("自分の uid が分からなければ隠す", others.hides(c, null))
        assertFalse(others.hides(c, "u1"))
        assertFalse("private(作者なし)は出す", FormDraft(item, Record("r2")).hides(c, "u2"))
    }

    @Test
    fun screenFormLoadsEditsAndPrefills() {
        val r = Record("r1", mapOf("name" to JSONValue.Str("既存")))
        val rt = FakeRuntime(entities, editable = false)
        assertNull("recordId が一覧に無ければ新規", ScreenForm.formRecord(item, "zzz", listOf(r)))
        val edit = ScreenForm.makeDraft(item, ScreenForm.formRecord(item, "r1", listOf(r)), rt)
        assertEquals("r1", edit.recordId)
        assertTrue("他人の記録は読むだけ", edit.isReadOnly)
        assertFalse(edit.hasUnsavedChanges)

        FormPrefill.set(mapOf("shop" to JSONValue.Str("s1"), "nope" to JSONValue.Str("x")))
        val fresh = ScreenForm.makeDraft(item, null, rt)
        assertEquals(JSONValue.Str("s1"), fresh.values["shop"])
        assertFalse("entity に無い field は捨てる", fresh.values.containsKey("nope"))
        assertFalse("積まれた初期値は入力ではない", fresh.hasUnsavedChanges)

        val profile = entities.first { it.id == "profile" }
        val one = Record("p1", mapOf("nick" to JSONValue.Str("たろう")))
        assertEquals("single は recordId によらずその 1 件", "p1", ScreenForm.formRecord(profile, null, listOf(one))?.id)
        assertTrue(ScreenForm.keeps(edit, item, r))
        assertFalse(ScreenForm.keeps(edit, item, null))
    }

    @Test
    fun dailyToggleDateHasNoDefault() {
        val d = AppDSL.decode(
            """
            {"schemaVersion":1,"app":{"name":"t","voiceEntry":"none"},"initialScreen":"home",
             "entities":[{"id":"m","name":"薬","fields":[{"id":"name","name":"名前","type":"string"},{"id":"taken","name":"飲んだ日","type":"date"}]}],
             "screens":[{"id":"home","title":"h","components":[{"type":"list","entity":"m","item":{"title":"name","toggle":"taken","resets":"daily"}}]}]}
            """.trimIndent(),
        )
        val f = FormDraft(d.entityModels().first())
        assertFalse("追加した瞬間に「済み」にしない(#132)", f.payload.containsKey("taken"))
    }

    @Test
    fun numericAndTextRules() {
        assertEquals(10.0..10.001, NumericInput.range(10.0, 5.0, 0.0, 100.0))
        assertEquals(0.0..100.0, NumericInput.range(null, Double.NaN, 0.0, 100.0))
        assertNull(NumericInput.step(0.0))
        assertEquals(5, NumericInput.ratingCount(null))
        assertEquals(10, NumericInput.ratingCount(1e300))
        assertEquals(3, NumericInput.ratingValue(2.5, 5))
        assertEquals(0, NumericInput.ratingValue(Double.NaN, 5))
        assertEquals(9, NumericInput.sliderSteps(0.0..10.0, 1.0))
        assertEquals(4.0, NumericInput.snap(3.6, 0.0..10.0, 2.0), 0.0)

        assertEquals(4..10, TextInputRules.editorLines(null))
        assertEquals(30..36, TextInputRules.editorLines(Int.MAX_VALUE))
        val note = item.field("note")
        assertTrue("メモ系の名前は複数行", TextInputRules.isMultiline(Component(type = ComponentType.TEXT_FIELD), note, "name"))
        assertFalse(TextInputRules.isMultiline(Component(type = ComponentType.TEXT_FIELD, multiline = false), note, "name"))

        assertEquals(listOf("A", "B", "C"), PickerRules.options(listOf("A", "B"), "C"))
        assertEquals("segmented", PickerRules.style(Component(type = ComponentType.PICKER), 4))
        assertEquals("menu", PickerRules.style(Component(type = ComponentType.PICKER), 5))
        assertEquals("wheel", PickerRules.style(Component(type = ComponentType.PICKER, style = ComponentStyle.WHEEL), 2))
    }
}
