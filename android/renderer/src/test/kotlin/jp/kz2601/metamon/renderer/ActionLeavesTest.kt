package jp.kz2601.metamon.renderer

import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.dsl.DslJson
import jp.kz2601.metamon.dsl.expr.Expr
import jp.kz2601.metamon.dsl.expr.ExprEvaluator
import jp.kz2601.metamon.dsl.records.FieldCoding
import jp.kz2601.metamon.dsl.records.Record
import java.time.Instant
import jp.kz2601.metamon.dsl.logic.OpenApp
import jp.kz2601.metamon.dsl.logic.ShareTarget
import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.Field
import jp.kz2601.metamon.dsl.records.FieldType
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.renderer.components.actions.ActionButtonKind
import jp.kz2601.metamon.renderer.components.actions.ExternalOpen
import jp.kz2601.metamon.renderer.components.actions.FormTemplate
import jp.kz2601.metamon.renderer.components.actions.OpenUrlTarget
import jp.kz2601.metamon.renderer.components.actions.QrContent
import jp.kz2601.metamon.renderer.components.actions.QrMatrix
import jp.kz2601.metamon.renderer.components.actions.ShareAction
import jp.kz2601.metamon.renderer.components.actions.WebViewNavigation
import jp.kz2601.metamon.renderer.components.buttonDefaultIcon
import jp.kz2601.metamon.renderer.form.FormDraft
import jp.kz2601.metamon.renderer.registry.ComponentRegistry
import jp.kz2601.metamon.dsl.Action
import jp.kz2601.metamon.dsl.ActionType
import jp.kz2601.metamon.dsl.ComponentStyle
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNotNull
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test
import java.net.URI
import java.net.URLDecoder

/** #861: 外へ出る Leaf の決まり(開く URL・共有の文面・QR・webView の移動)。描画は見ない。 */
class ActionLeavesTest {
    private val entity = Entity(
        id = "shop",
        fields = listOf(
            Field("name", type = FieldType.STRING),
            Field("tel", type = FieldType.STRING),
            Field("photo", type = FieldType.PHOTO),
        ),
    )

    private fun form(vararg values: Pair<String, String>) = FormDraft(entity).also { f ->
        values.forEach { (k, v) -> f.values[k] = JSONValue.Str(v) }
    }

    @Test
    fun registeredInNameOrder() {
        val types = ComponentRegistry.standard.registeredTypes
        for (t in listOf(ComponentType.BUTTON, ComponentType.OPEN_URL_BUTTON, ComponentType.QR_CODE, ComponentType.SHARE_BUTTON, ComponentType.WEB_VIEW)) {
            assertTrue(t.wire, t.wire in types)
        }
    }

    @Test
    fun buttonKindsAndIcons() {
        assertEquals(ActionButtonKind.PRIMARY, ActionButtonKind.of(null))
        assertEquals(ActionButtonKind.DESTRUCTIVE, ActionButtonKind.of(ComponentStyle.DESTRUCTIVE))
        assertEquals(ActionButtonKind.SECONDARY, ActionButtonKind.of(ComponentStyle.TITLE, default = ActionButtonKind.SECONDARY))
        assertEquals("square.and.arrow.up", buttonDefaultIcon(Action(type = ActionType.EXPORT)))
        assertNull(buttonDefaultIcon(Action(type = ActionType.SAVE_RECORD)))
    }

    // openUrlButton

    @Test
    fun openUrlButtonUrlNeedsAllowedScheme() {
        assertEquals(OpenUrlTarget.Url("https://jphacks.com"), OpenUrlTarget.resolve(null, null, "https://jphacks.com", null))
        assertEquals(OpenUrlTarget.Url("tel:0761234567"), OpenUrlTarget.resolve(null, null, "tel:0761234567", null))
        assertEquals(OpenUrlTarget.Invalid, OpenUrlTarget.resolve(null, null, "javascript:alert(1)", null))
        assertEquals(OpenUrlTarget.Invalid, OpenUrlTarget.resolve(null, null, "http://example.com", null))
        assertEquals(OpenUrlTarget.Invalid, OpenUrlTarget.resolve(null, null, null, null))
    }

    @Test
    fun openUrlButtonAppWins() {
        assertEquals(
            OpenUrlTarget.App(OpenApp.X, listOf("https://x.com/jphacks")),
            OpenUrlTarget.resolve("x", "@jphacks", "https://example.com", null),
        )
        assertEquals("知らない app は url があっても開かない(iOS と同じ)", OpenUrlTarget.Invalid, OpenUrlTarget.resolve("telegram", null, "https://example.com", null))
    }

    @Test
    fun openUrlButtonToReadsTheRecord() {
        val t = OpenUrlTarget.resolve("phone", "{{tel}}", null, form("tel" to "076-123-4567"))
        assertEquals(OpenUrlTarget.App(OpenApp.PHONE, listOf("tel:0761234567")), t)
        val empty = OpenUrlTarget.resolve("phone", "{{tel}}", null, form())
        assertEquals("番号がまだ無い = 押せない", emptyList<String>(), (empty as OpenUrlTarget.App).urls)
    }

    @Test
    fun candidatesFallBackAndMapToAndroid() {
        assertEquals(listOf("slack://open", "https://slack.com/"), ExternalOpen.candidates(OpenApp.SLACK.urls(null)))
        assertEquals("Apple の地図は Google マップへ", listOf("https://www.google.com/maps/"), ExternalOpen.candidates(OpenApp.MAPS.urls(null)))
        assertEquals(listOf("https://www.google.com/maps/search/?api=1&query=Kanazawa"), ExternalOpen.candidates(listOf("maps:?q=Kanazawa")))
        assertEquals("許可していないスキームは落とす", emptyList<String>(), ExternalOpen.candidates(listOf("javascript:alert(1)", "intent://x")))
        assertTrue(ExternalOpen.usesCustomTab("https://jphacks.com", preferApp = false))
        assertFalse("app で開く https は対応アプリへ", ExternalOpen.usesCustomTab("https://x.com/jphacks", preferApp = true))
        assertFalse(ExternalOpen.usesCustomTab("tel:1", preferApp = false))
    }

    // shareButton

    @Test
    fun shareTextFillsTemplateAndField() {
        val f = form("name" to "麺屋", "tel" to "0761")
        assertEquals("麺屋 に行った\n0761", FormTemplate.shareText("{{name}} に行った", "tel", f))
        assertEquals("写真の field は文面に足さない", "麺屋", FormTemplate.shareText("{{name}}", "photo", f))
        assertEquals("フォームの外はそのまま", "{{name}}", FormTemplate.shareText("{{name}}", "tel", null))
    }

    @Test
    fun shareActionPerTarget() {
        val f = form("name" to "麺屋")
        assertEquals(ShareAction.Sheet("麺屋"), ShareAction.of(ShareTarget.ANY, "{{name}}", null, f))
        assertEquals(ShareAction.Sheet("💬 今日"), ShareAction.of(ShareTarget.SLACK, "**今日**", null, null))
        val line = ShareAction.of(ShareTarget.LINE, "{{name}} & 友だち", null, f) as ShareAction.Direct
        val q = URI(line.url).rawQuery
        assertEquals("麺屋 & 友だち", URLDecoder.decode(q.substringAfter("text="), Charsets.UTF_8))
        assertEquals("開けなければ共有シートに同じ文面", "麺屋 & 友だち", line.text)
        assertTrue((ShareAction.of(ShareTarget.MAIL, "a", null, null) as ShareAction.Direct).url.startsWith("mailto:?body="))
    }

    // qrCode

    @Test
    fun qrMatrixHasQuietZoneAndLevelM() {
        val m = notNull(QrMatrix.encode("https://example.com"))
        // バージョン 2(25 モジュール)+ 余白 4 × 2
        assertEquals(25 + 8, m.size)
        for (i in 0 until m.size) {
            for (q in 0 until QrMatrix.QUIET_ZONE) {
                assertFalse(m[i, q])
                assertFalse(m[q, i])
                assertFalse(m[i, m.size - 1 - q])
                assertFalse(m[m.size - 1 - q, i])
            }
        }
        // 左上の位置検出パターンの角
        assertTrue(m[4, 4])
        assertTrue(m[10, 10])
        assertFalse(m[5, 5])
    }

    @Test
    fun qrMatrixEmptyAndTooLong() {
        assertNull(QrMatrix.encode(""))
        assertNull("バージョン 40 に入らない", QrMatrix.encode("a".repeat(3000)))
        assertNotNull("日本語も入る", QrMatrix.encode("金沢駅"))
    }

    @Test
    fun qrContent() {
        val f = form("name" to "麺屋")
        assertEquals("店: 麺屋", QrContent.resolve("店: {{name}}", null, null, f, noEvaluator))
        assertEquals("麺屋", QrContent.resolve(null, "name", null, f, noEvaluator))
        assertEquals("", QrContent.resolve(null, "name", null, null, noEvaluator))
        assertEquals("", QrContent.resolve(null, null, null, f, noEvaluator))
    }

    private val noEvaluator: () -> ExprEvaluator = { throw AssertionError("式が無いのに評価器を作った") }

    private val order = Entity(
        id = "order",
        fields = listOf(
            Field("name", type = FieldType.STRING),
            Field("price", type = FieldType.NUMBER),
            Field("qty", type = FieldType.NUMBER),
            Field("start", type = FieldType.DATETIME),
            Field("total", type = FieldType.NUMBER, computed = Expr.parse("""{"op":"mul","args":[{"ref":"price"},{"ref":"qty"}]}""")),
            Field("end", type = FieldType.DATETIME, computed = Expr.parse("""{"op":"addMinutes","args":[{"ref":"start"},30]}""")),
        ),
    )

    private fun orderEvaluator(vararg records: Record): () -> ExprEvaluator =
        { ExprEvaluator(listOf(order), { if (it == "order") records.toList() else emptyList() }) }

    private fun orderForm(): FormDraft = FormDraft(order).also { f ->
        f.values["name"] = JSONValue.Str("らーめん")
        f.values["price"] = JSONValue.Num(800.0)
        f.values["qty"] = JSONValue.Num(3.0)
        f.values["start"] = JSONValue.Str("2026-11-11T01:00:00Z")
    }

    @Test
    fun qrContentComputedFieldEvaluatesDraft() {
        val f = orderForm()
        val ev = orderEvaluator()
        assertEquals("2400", QrContent.resolve(null, "total", null, f, ev))
        assertEquals("合計 2400 円", QrContent.resolve("合計 {{total}} 円", null, null, f, ev))
        // datetime の computed は RFC3339(時刻を落とさない)
        val end = FieldCoding.formatDatetime(Instant.parse("2026-11-11T01:30:00Z"))
        assertEquals(end, QrContent.resolve(null, "end", null, f, ev))
        // 保存値はそのまま(整形しない)
        assertEquals("2026-11-11T01:00:00Z", QrContent.resolve(null, "start", null, f, ev))
        // 下書きを変えると追従する
        f.values["qty"] = JSONValue.Num(2.0)
        assertEquals("1600", QrContent.resolve(null, "total", null, f, ev))
    }

    @Test
    fun qrContentValueExpr() {
        val f = orderForm()
        val concat = Expr.parse("""{"op":"concat","args":["注文: ",{"ref":"name"}," x",{"ref":"total"}]}""")
        assertEquals("注文: らーめん x2400", QrContent.resolve(null, null, concat, f, orderEvaluator()))
        // フォームの外: 行なしで評価(集計は読める・ref は空)
        val count = Expr.parse("""{"agg":"count","entity":"order"}""")
        val ev = orderEvaluator(Record("a", mapOf()), Record("b", mapOf()))
        assertEquals("2", QrContent.resolve(null, null, count, null, ev))
        assertEquals("", QrContent.resolve(null, null, Expr.parse("""{"ref":"name"}"""), null, ev))
        // text / field が先
        assertEquals("固定", QrContent.resolve("固定", null, concat, f, orderEvaluator()))
        assertEquals("らーめん", QrContent.resolve(null, "name", concat, f, orderEvaluator()))
    }

    @Test
    fun qrContentReadsComponentValue() {
        val c = DslJson.decodeFromString(Component.serializer(), """{"type":"qrCode","value":{"op":"add","args":[1,2]}}""")
        assertEquals("3", QrContent.resolve(c, null, orderEvaluator()))
    }

    // webView

    @Test
    fun webViewNavigation() {
        assertEquals(WebViewNavigation.Decision.ALLOW, WebViewNavigation.decide("https://jphacks.com/a"))
        assertEquals(WebViewNavigation.Decision.ALLOW, WebViewNavigation.decide("about:blank"))
        assertEquals(WebViewNavigation.Decision.OPEN_OUTSIDE, WebViewNavigation.decide("tel:0761234567"))
        assertEquals(WebViewNavigation.Decision.OPEN_OUTSIDE, WebViewNavigation.decide("line://nv/chat"))
        assertEquals(WebViewNavigation.Decision.BLOCK, WebViewNavigation.decide("intent://scan/#Intent;scheme=zxing;end"))
        assertEquals(WebViewNavigation.Decision.BLOCK, WebViewNavigation.decide("file:///etc/hosts"))
        assertEquals(WebViewNavigation.Decision.BLOCK, WebViewNavigation.decide("javascript:alert(1)"))
    }

    private fun <T : Any> notNull(v: T?): T {
        org.junit.Assert.assertNotNull(v)
        return v!!
    }
}
