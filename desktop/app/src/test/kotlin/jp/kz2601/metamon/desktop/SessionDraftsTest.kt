package jp.kz2601.metamon.desktop

import jp.kz2601.metamon.dsl.Action
import jp.kz2601.metamon.dsl.ActionType
import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.decode
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.renderer.form.FormPrefill
import jp.kz2601.metamon.renderer.navigation.Route
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNotSame
import org.junit.Assert.assertNull
import org.junit.Assert.assertSame
import org.junit.Assert.assertTrue
import org.junit.Test

/** フォームの下書き(閉じたら捨てる・記録が変わったら作り直す・FormPrefill)と、一覧のチェック・削除の記録の規則。 */
class SessionDraftsTest {
    private fun shopping(store: MockStore = MockStore(), uid: String? = "me") = DesktopSession(Examples.load("shopping"), store, uid = uid)

    private fun shoppingText(): String =
        javaClass.classLoader.getResourceAsStream("examples/shopping.json")!!.use { it.readBytes().decodeToString() }

    /** shopping の groceries を共有(作った人だけが変えられる)にした物。 */
    private fun sharedShopping(): AppDSL = AppDSL.decode(shoppingText().replaceFirst("\"titleField\": \"name\"", "\"scope\": \"shared\", \"titleField\": \"name\""))

    private val add = Action(type = ActionType.NAVIGATE, screen = "item_form", presentation = jp.kz2601.metamon.dsl.Presentation.SHEET)

    private fun tapRow(s: DesktopSession, i: Int = 0) =
        assertTrue(s.apply(JsonObject(mapOf("tapRow" to JsonObject(mapOf("index" to JsonPrimitive(i)))))))

    @Test
    fun reopenedNewFormStartsEmpty() {
        val s = shopping()
        s.dispatch(add, ActionContext(s.top, null))
        s.form(s.top)!!.values["name"] = JSONValue.Str("牛乳")
        s.back()
        assertEquals("home", s.top.screen)
        assertFalse("閉じた sheet の下書きは捨てる", s.draftRouteIds.any { it.startsWith(Route.SHEET) })
        s.dispatch(add, ActionContext(s.top, null))
        assertEquals("", s.form(s.top)!!.values["name"]?.stringValue.orEmpty())
    }

    @Test
    fun reopenedEditFormDropsUnsavedEdits() {
        val s = shopping(MockStore().save("groceries", null, mapOf("name" to "牛乳", "done" to "false")))
        tapRow(s)
        s.form(s.top)!!.values["name"] = JSONValue.Str("書きかけ")
        s.back()
        tapRow(s)
        assertEquals("牛乳", s.form(s.top)!!.values["name"]?.stringValue)
    }

    @Test
    fun draftIsRebuiltWhenTheRecordChanges() {
        val s = shopping(MockStore().save("groceries", null, mapOf("name" to "牛乳", "done" to "false")))
        tapRow(s)
        val before = s.form(s.top)!!
        assertSame("変わっていなければ同じ下書き", before, s.form(s.top))
        s.replaceStore(s.store.save("groceries", "new-1", mapOf("name" to "豆乳", "done" to "false")))
        val after = s.form(s.top)!!
        assertNotSame(before, after)
        assertEquals("豆乳", after.values["name"]?.stringValue)
        // 打ちかけの入力がある時は残す
        after.values["name"] = JSONValue.Str("書きかけ")
        s.replaceStore(s.store.save("groceries", "new-1", mapOf("name" to "アーモンドミルク", "done" to "false")))
        assertEquals("書きかけ", s.form(s.top)!!.values["name"]?.stringValue)
    }

    @Test
    fun prefillIsTakenByTheFreshFormOnly() {
        val s = shopping()
        // 前に開いた新規フォームの下書きが残っていても、開き直した新規は初期値を受け取る
        s.dispatch(add, ActionContext(s.top, null))
        s.back()
        FormPrefill.set(mapOf("category" to JSONValue.Str("日用品")))
        s.dispatch(add, ActionContext(s.top, null))
        assertEquals("日用品", s.form(s.top)!!.values["category"]?.stringValue)
        // 受け取った後は残らない(次の新規に漏れない)
        s.back()
        s.dispatch(add, ActionContext(s.top, null))
        assertEquals("食品", s.form(s.top)!!.values["category"]?.stringValue)
    }

    @Test
    fun prefillDoesNotLeakPastAScreenWithoutANewForm() {
        val s = shopping(MockStore().save("groceries", null, mapOf("name" to "牛乳", "done" to "false")))
        FormPrefill.set(mapOf("category" to JSONValue.Str("日用品")))
        tapRow(s) // 既存の記録の編集(新規ではない)
        s.back()
        s.dispatch(add, ActionContext(s.top, null))
        assertEquals("食品", s.form(s.top)!!.values["category"]?.stringValue)
    }

    @Test
    fun listToggleAndDeleteFollowTheRecordRules() {
        val store = MockStore(
            mapOf(
                "groceries" to listOf(
                    Row("mine", mapOf("name" to "牛乳", "done" to "false"), author = "me"),
                    Row("theirs", mapOf("name" to "卵", "done" to "false"), author = "someone"),
                ),
            ),
        )
        val s = DesktopSession(sharedShopping(), store, uid = "me")
        assertTrue(s.canEditRecord("groceries", "mine"))
        assertFalse(s.canEditRecord("groceries", "theirs"))
        assertTrue(s.updateRecord("groceries", "mine", mapOf("done" to "true")))
        assertEquals("true", s.store.find("groceries", "mine")!!.data["done"])
        assertFalse(s.updateRecord("groceries", "theirs", mapOf("done" to "true")))
        assertEquals("false", s.store.find("groceries", "theirs")!!.data["done"])
        assertNotNull("書けない理由", s.message)

        val delete = Action(type = ActionType.DELETE_RECORD, entity = "groceries")
        assertFalse(s.canDeleteRecord("groceries", "theirs"))
        s.dispatch(delete, ActionContext(s.top, null, "theirs"))
        assertNotNull("消せない記録は残す", s.store.find("groceries", "theirs"))
        assertNotNull(s.message)
        s.dispatch(delete, ActionContext(s.top, null, "mine"))
        assertNull(s.store.find("groceries", "mine"))
    }

    @Test
    fun privateEntityKeepsListToggleAndDelete() {
        val s = shopping(MockStore().save("groceries", null, mapOf("name" to "牛乳", "done" to "false")))
        assertTrue(s.updateRecord("groceries", "new-1", mapOf("done" to "true")))
        s.dispatch(Action(type = ActionType.DELETE_RECORD, entity = "groceries"), ActionContext(s.top, null, "new-1"))
        assertTrue(s.records("groceries").isEmpty())
    }
}
