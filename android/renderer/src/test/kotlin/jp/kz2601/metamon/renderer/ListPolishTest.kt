package jp.kz2601.metamon.renderer

import jp.kz2601.metamon.dsl.ActionType
import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.DslJson
import jp.kz2601.metamon.dsl.decode
import jp.kz2601.metamon.dsl.records.Field
import jp.kz2601.metamon.dsl.records.FieldType
import jp.kz2601.metamon.dsl.screen
import jp.kz2601.metamon.renderer.components.lists.ChartReadout
import jp.kz2601.metamon.renderer.components.lists.EmptyCta
import jp.kz2601.metamon.renderer.components.lists.NewRowHighlight
import jp.kz2601.metamon.renderer.components.lists.RecordsPlaceholder
import jp.kz2601.metamon.renderer.components.lists.StatIcon
import jp.kz2601.metamon.renderer.screen.ScreenToolbar
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test
import java.util.Locale

/** 一覧の仕上げ(#1118): 新しい行のハイライト・空の一覧の追加・stat の既定アイコン・グラフの棒ごとの読み上げ。 */
class ListPolishTest {
    // --- 新しい行のハイライト(iOS の markNewRows)

    @Test
    fun highlightsFirstNewRowOnly() {
        assertEquals("c", NewRowHighlight.fresh(setOf("a", "b"), listOf("a", "c", "b", "d"), reduceMotion = false))
    }

    @Test
    fun noHighlightOnFirstDrawOrFirstLoadOrReduceMotion() {
        assertNull("初回の描画", NewRowHighlight.fresh(null, listOf("a"), reduceMotion = false))
        assertNull("空からの初回ロード", NewRowHighlight.fresh(emptySet(), listOf("a"), reduceMotion = false))
        assertNull("動きを減らす設定", NewRowHighlight.fresh(setOf("a"), listOf("a", "b"), reduceMotion = true))
        assertNull("増えていない(削除・並べ替え)", NewRowHighlight.fresh(setOf("a", "b"), listOf("b"), reduceMotion = false))
        assertEquals(1_200L, NewRowHighlight.MILLIS)
    }

    // --- 空の一覧の「最初の 1 件を追加しましょう」(iOS の DuplicateAddControl.emptyListAdd)

    private fun dsl(toolbar: String, components: String): AppDSL = AppDSL.decode(
        """
        {"schemaVersion":1,"app":{"name":"t"},"initialScreen":"home",
         "entities":[{"id":"log","name":"記録","fields":[{"id":"title","name":"内容","type":"string"}]}],
         "screens":[
          {"id":"home","title":"ホーム","toolbar":[$toolbar],"components":[$components]},
          {"id":"settings","title":"設定","components":[]},
          {"id":"form","title":"追加","entity":"log","components":[]}]}
        """,
    )

    private val plus = """{"label":"追加","icon":"plus","action":{"type":"navigate","screen":"form","presentation":"sheet"}}"""
    private val addButton = """{"type":"button","label":"記録を追加","action":{"type":"navigate","screen":"form","presentation":"sheet"}}"""

    private fun emptyAdd(d: AppDSL, entity: String = "log") = ScreenToolbar.emptyListAdd(entity, d.screen("home")!!, d)

    @Test
    fun emptyListAddUsesSameScreenAddControl() {
        val a = emptyAdd(dsl("", addButton))
        assertEquals(ActionType.NAVIGATE, a?.type)
        assertEquals("同じ画面の「記録を追加」と同じフォーム", "form", a?.screen)
        assertEquals("button が無ければ右上の＋と同じ", "form", emptyAdd(dsl(plus, ""))?.screen)
    }

    @Test
    fun emptyListAddIsNullWithoutAddControl() {
        assertNull("追加の入口が無い", emptyAdd(dsl("", "")))
        val settings = """{"type":"button","label":"設定","action":{"type":"navigate","screen":"settings"}}"""
        assertNull("フォーム以外への移動は使わない", emptyAdd(dsl("", settings)))
        assertNull("別の entity のフォームは使わない", emptyAdd(dsl("", addButton), entity = "other"))
    }

    @Test
    fun emptyCtaOnlyWhenTrulyEmpty() {
        assertEquals(EmptyCta.BUTTON, EmptyCta.of(RecordsPlaceholder.EMPTY, canAdd = true))
        assertEquals("追加の入口が無ければ文字だけ(iOS と同じ)", EmptyCta.TEXT, EmptyCta.of(RecordsPlaceholder.EMPTY, canAdd = false))
        assertEquals(EmptyCta.NONE, EmptyCta.of(RecordsPlaceholder.SKELETON, canAdd = true))
        assertEquals(EmptyCta.NONE, EmptyCta.of(RecordsPlaceholder.UNAVAILABLE, canAdd = true))
    }

    // --- stat の既定アイコン(iOS の FieldIcon.stat)

    private fun c(json: String): Component = DslJson.decodeFromString(Component.serializer(), json)

    @Test
    fun statIconDefaults() {
        val price = Field("price", "予算", FieldType.CURRENCY)
        assertEquals("number", StatIcon.name(c("""{"type":"stat","entity":"log","aggregate":"count"}"""), null))
        assertEquals("chart.bar", StatIcon.name(c("""{"type":"stat","entity":"log","aggregate":"avg","field":"price"}"""), price))
        assertEquals("sum は項目の型", "yensign", StatIcon.name(c("""{"type":"stat","entity":"log","aggregate":"sum","field":"price"}"""), price))
        assertEquals("指定が優先", "flame", StatIcon.name(c("""{"type":"stat","entity":"log","aggregate":"count","icon":"flame"}"""), null))
        assertNull("計算式モードで未指定なら出さない(#193)", StatIcon.name(c("""{"type":"stat","value":{"op":"div","args":[1,0]}}"""), null))
    }

    @Test
    fun fieldIconInferenceFollowsIos() {
        assertEquals("scalemass", StatIcon.infer(Field("w", "体重", FieldType.NUMBER), "体重"))
        assertEquals("figure.walk", StatIcon.infer(Field("steps", "歩数", FieldType.INTEGER), "歩数"))
        assertEquals("clock", StatIcon.infer(Field("t", "開始時刻", FieldType.DATETIME), "開始時刻"))
        assertEquals("note.text", StatIcon.infer(Field("memo", "メモ", FieldType.STRING), "メモ"))
        assertEquals("pencil.line", StatIcon.infer(null, ""))
    }

    // --- グラフの棒・点ごとの読み上げ

    @Test
    fun chartPointReadsLabelAndValue() {
        assertEquals("3月, 1,200", ChartReadout.point("3月", 1200.0, Locale.JAPAN))
        assertEquals("食費, 2.5", ChartReadout.point("食費", 2.5, Locale.JAPAN))
    }
}
