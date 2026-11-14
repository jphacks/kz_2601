package jp.kz2601.metamon.desktop

import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.entityModels
import jp.kz2601.metamon.renderer.form.FormDraft
import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

/** InputLeaves.caption / options(#3200): 見出しは label → 項目の名前 → 項目の id。選択肢は今の値が外れていれば足す。 */
class InputLeaves3200Test {
    private val member = Examples.load("file_import").entityModels().first { it.id == "member" }

    private fun form() = FormDraft(member)

    @Test
    fun captionFallsBackToFieldName() {
        val f = form()
        // label があればそれ
        assertEquals("お名前", InputLeaves.caption(Component(type = ComponentType.TEXT_FIELD, field = "name", label = "お名前"), f))
        // label が無ければ項目の名前(toggle・date など補う種類)
        assertEquals("入部日", InputLeaves.caption(Component(type = ComponentType.DATE_PICKER, field = "joined"), f))
        // 項目が DSL に無ければ項目の id
        assertEquals("ghost", InputLeaves.caption(Component(type = ComponentType.DATE_PICKER, field = "ghost"), f))
        // 補わない種類(textField)は label が無いと項目の名前、無ければ id になる(見出しだけの補い)
        assertEquals("名前", InputLeaves.caption(Component(type = ComponentType.TEXT_FIELD, field = "name"), f))
        assertEquals("ghost", InputLeaves.caption(Component(type = ComponentType.TEXT_FIELD, field = "ghost"), f))
        // field が無ければ空
        assertEquals("", InputLeaves.caption(Component(type = ComponentType.TEXT_FIELD), f))
        assertEquals("見出し", InputLeaves.caption(Component(type = ComponentType.TEXT_FIELD, label = "見出し"), f))
    }

    @Test
    fun optionsKeepsCurrentValue() {
        val f = form()
        val base = InputLeaves.options(f, "part")
        assertTrue("enum の選択肢がある", base.isNotEmpty())
        // 選択肢の中の値は足さない
        f.values["part"] = JSONValue.Str(base.first())
        assertEquals(base, InputLeaves.options(f, "part"))
        // 外れた今の値は末尾に足す
        f.values["part"] = JSONValue.Str("引退")
        assertEquals(base + "引退", InputLeaves.options(f, "part"))
        // 空の値は足さない
        f.values["part"] = JSONValue.Str("")
        assertEquals(base, InputLeaves.options(f, "part"))
        // 項目が無ければ選択肢は無い・今の値だけ入る
        f.values["ghost"] = JSONValue.Str("x")
        assertEquals(listOf("x"), InputLeaves.options(f, "ghost"))
        assertEquals(emptyList<String>(), InputLeaves.options(f, "missing"))
    }
}
