package jp.kz2601.metamon.runtime

import jp.kz2601.metamon.dsl.Action
import jp.kz2601.metamon.dsl.ActionType
import jp.kz2601.metamon.dsl.DslJson
import jp.kz2601.metamon.dsl.Presentation
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.renderer.ActionContext
import jp.kz2601.metamon.renderer.form.FormDraft
import jp.kz2601.metamon.renderer.form.FormPrefill
import jp.kz2601.metamon.renderer.navigation.Route
import jp.kz2601.metamon.runtime.RuntimeFixture.runtime
import kotlinx.coroutines.test.runTest
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import org.junit.After
import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Test
import java.io.File

/** iOS の PrefillRefNavigationTests(#256): navigate.prefillRef で今の記録を子フォームの ref に積み、子は新規で開く。 */
class PrefillRefNavigationTest {
    private val dsl = RuntimeFixture.dsl("post_comments")

    /** post_form の「コメントを書く」ボタンのアクション(正典の例から読む)。 */
    private fun writeComment(): Action {
        val root = Json.parseToJsonElement(File(System.getProperty("metamon.examplesDir"), "post_comments.json").readText()) as JsonObject
        val screen = (root["screens"] as JsonArray).map { it as JsonObject }.first { (it["id"] as JsonPrimitive).content == "post_form" }
        val button = (screen["components"] as JsonArray).map { it as JsonObject }
            .first { (it["id"] as? JsonPrimitive)?.content == "write_comment" }
        return DslJson.decodeFromJsonElement(Action.serializer(), button["action"]!!)
    }

    @After
    fun tearDown() {
        // 次のテストに古い初期値を残さない
        FormPrefill.set(emptyMap())
    }

    @Test
    fun buttonOnSavedParentPrefillsRefAndOpensNewChild() = runTest {
        val rt = runtime(dsl)
        val posts = rt.entityModel("posts")!!
        val draft = FormDraft(posts, Record("p1", mapOf("body" to JSONValue.Str("こんにちは"))))
        val ctx = ActionContext(Route("post_form", recordId = "p1", stack = Route.SHEET), form = draft)

        rt.run(writeComment(), ctx)

        val sheet = rt.navigation.value.sheet
        assertNotNull("子のフォームがシートで開く", sheet)
        val opened = sheet!!
        assertEquals("comment_form", opened.screen)
        assertNull("子は新規(親の id を recordId に渡さない)", opened.recordId)
        assertNull(rt.toast)
        val comments = rt.entityModel("post_comments")!!
        assertEquals(mapOf("post" to JSONValue.Str("p1")), FormPrefill.consume(comments))
        assertEquals("1 回だけ消費される", emptyMap<String, JSONValue>(), FormPrefill.consume(comments))
    }

    @Test
    fun rowTapPrefillsTappedRecord() = runTest {
        val rt = runtime(dsl)
        val action = Action(type = ActionType.NAVIGATE, screen = "comment_form", presentation = Presentation.SHEET, prefillRef = "post")
        val ctx = ActionContext(Route("home", stack = Route.tabStack("home")), recordId = "p2")

        rt.run(action, ctx)

        assertNull(rt.navigation.value.sheet?.recordId)
        assertEquals("comment_form", rt.navigation.value.sheet?.screen)
        assertEquals(mapOf("post" to JSONValue.Str("p2")), FormPrefill.consume(rt.entityModel("post_comments")!!))
    }

    @Test
    fun unsavedParentShowsToastAndStays() = runTest {
        val rt = runtime(dsl)
        val posts = rt.entityModel("posts")!!
        val ctx = ActionContext(Route("post_form", stack = Route.SHEET), form = FormDraft(posts))

        rt.run(writeComment(), ctx)

        assertNull("親が未保存なら子のフォームを開かない", rt.navigation.value.sheet)
        assertEquals("先に「${posts.name}」を保存してください", rt.toast)
        assertEquals(emptyMap<String, JSONValue>(), FormPrefill.consume(rt.entityModel("post_comments")!!))
    }

    @Test
    fun navigateWithoutPrefillRefKeepsRecordId() = runTest {
        val rt = runtime(dsl)
        val action = Action(type = ActionType.NAVIGATE, screen = "post_form", presentation = Presentation.SHEET)
        val ctx = ActionContext(Route("home", stack = Route.tabStack("home")), recordId = "p3")

        rt.run(action, ctx)

        assertEquals("従来どおり行の id で編集を開く", "p3", rt.navigation.value.sheet?.recordId)
        assertEquals(emptyMap<String, JSONValue>(), FormPrefill.consume(rt.entityModel("posts")!!))
    }
}
