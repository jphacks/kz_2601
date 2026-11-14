package jp.kz2601.metamon.renderer

import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.dsl.TextHighlight
import jp.kz2601.metamon.dsl.expr.Expr
import jp.kz2601.metamon.dsl.expr.ExprEvaluator
import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.Field
import jp.kz2601.metamon.dsl.records.FieldType
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.renderer.components.TextContent
import jp.kz2601.metamon.renderer.components.highlighted
import jp.kz2601.metamon.renderer.form.FormDraft
import org.junit.Assert.assertEquals
import org.junit.Test

/** #1243: text の `{{fieldId}}` と field からの highlight(iOS の `TextContent`・`TextLeaf`)。 */
class TextContentTest {
    private val task = Entity(
        id = "task",
        fields = listOf(
            Field("title", type = FieldType.STRING),
            Field("price", type = FieldType.NUMBER),
            Field("qty", type = FieldType.NUMBER),
            Field("secret", type = FieldType.STRING, hidden = true),
            Field("photo", type = FieldType.PHOTO),
            Field("tags", type = FieldType.TEXT),
            Field("total", type = FieldType.NUMBER, computed = Expr.parse("""{"op":"mul","args":[{"ref":"price"},{"ref":"qty"}]}""")),
        ),
    )
    private val evaluator: () -> ExprEvaluator = { ExprEvaluator(listOf(task), { emptyList() }) }

    private fun form() = FormDraft(task).also { f ->
        f.values["title"] = JSONValue.Str("買い物")
        f.values["price"] = JSONValue.Num(200.0)
        f.values["qty"] = JSONValue.Num(3.0)
        f.values["secret"] = JSONValue.Str("ひみつ")
        f.values["photo"] = JSONValue.Str("data:image/jpeg;base64,AAAA")
        f.values["tags"] = JSONValue.Str("牛乳、卵")
    }

    @Test
    fun fillsPlainAndComputedFields() {
        assertEquals("買い物 は 600 円", TextContent.resolve("{{title}} は {{total}} 円", form(), evaluator))
    }

    @Test
    fun leavesHiddenUnknownAndMediaAsWritten() {
        val f = form()
        assertEquals("{{secret}}", TextContent.resolve("{{secret}}", f, evaluator))
        assertEquals("{{photo}}", TextContent.resolve("{{photo}}", f, evaluator))
        assertEquals("{{nope}}", TextContent.resolve("{{nope}}", f, evaluator))
    }

    @Test
    fun outsideAFormKeepsTheText() {
        assertEquals("{{title}}", TextContent.resolve("{{title}}", null, evaluator))
    }

    @Test
    fun emptyValueBecomesEmpty() {
        val f = form()
        f.values.remove("title")
        assertEquals("「」", TextContent.resolve("「{{title}}」", f, evaluator))
    }

    @Test
    fun followsTheDraft() {
        val f = form()
        f.values["qty"] = JSONValue.Num(1.0)
        assertEquals("200", TextContent.resolve("{{total}}", f, evaluator))
    }

    @Test
    fun highlightTakesWordsFromTheFieldValue() {
        val c = Component(ComponentType.TEXT, text = "牛乳とパンと卵", highlight = TextHighlight(field = "tags"))
        val s = highlighted(c, fieldValue = "牛乳、卵")
        assertEquals("牛乳とパンと卵", s.text)
        assertEquals(listOf(0 until 2, 6 until 7), s.spanStyles.map { it.start until it.end })
        assertEquals("値が無ければ印なし", 0, highlighted(c, fieldValue = null).spanStyles.size)
    }

    @Test
    fun highlightMarksTheFilledText() {
        val c = Component(ComponentType.TEXT, text = "{{title}}", highlight = TextHighlight(words = listOf("買い物")))
        val s = highlighted(c, null, TextContent.resolve(c.text!!, form(), evaluator))
        assertEquals("買い物", s.text)
        assertEquals(1, s.spanStyles.size)
    }
}
