package jp.kz2601.metamon.renderer

import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.dsl.DslJson
import jp.kz2601.metamon.renderer.conformance.RenderTree
import jp.kz2601.metamon.renderer.navigation.NavigationResolver
import jp.kz2601.metamon.renderer.navigation.NavigationState
import jp.kz2601.metamon.renderer.navigation.Presentation
import jp.kz2601.metamon.renderer.navigation.rootRouteFor
import jp.kz2601.metamon.renderer.navigation.top
import jp.kz2601.metamon.renderer.registry.ComponentRegistry
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.booleanOrNull
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.doubleOrNull
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import org.junit.Assert.assertTrue
import org.junit.Assert.fail
import org.junit.Test
import java.io.File

/**
 * 適合テスト(#352)を Android で回す。docs/dsl/conformance/cases の case.json を読み、[RenderTree] の木を
 * iOS が書いた expected.json と比べる(expected.json は書き換えない)。
 *
 * #357 で比べるもの: 画面の id・title・route・layout、部品の id・type・cell・stacked・band・localGrid・children、
 * Container 7 種と Android に登録済みの Leaf の state・props・actions、遷移だけの操作(tap / tapToolbar / back)の後の top と画面。
 *
 * skip(数だけ出して赤にしない): まだ無い Leaf の state・props・actions(`unavailable` / `unknown` の state は比べる)、
 * toolbar・saveBar・form、フォーム・レコードに触れる操作(input・保存・削除・行タップ・prefillRef …)とその後の step。
 */
class ConformanceTest {
    private val docsDir = File(requireNotNull(System.getProperty("metamon.dslDocsDir")) { "metamon.dslDocsDir が未設定(Gradle から実行する)" })
    private val casesDir = File(docsDir, "conformance/cases")

    /** Android の描画器に登録済みの type(これ以外の Leaf は skip)。 */
    private val implemented = ComponentRegistry.standard.registeredTypes
    private val allKnown = ComponentType.entries.filter { it != ComponentType.UNKNOWN }.map { it.wire }.toSet()

    private class Report {
        val failures = mutableListOf<String>()
        val skipped = sortedMapOf<String, Int>()
        var steps = 0
        fun skip(what: String) { skipped[what] = (skipped[what] ?: 0) + 1 }
    }

    @Test
    fun containersAndGridMatchSharedCases() {
        val dirs = casesDir.listFiles { f -> File(f, "case.json").isFile && File(f, "expected.json").isFile }.orEmpty().sortedBy { it.name }
        assertTrue("case が見つからない: $casesDir", dirs.isNotEmpty())
        val report = Report()
        var compared = 0
        for (dir in dirs) {
            runCatching { runCase(dir, report) }.onFailure { report.failures += "${dir.name}: 例外 $it" }
            compared++
        }
        println("適合テスト: $compared case・遷移の step ${report.steps} 個を比較。skip: ${report.skipped}")
        if (report.failures.isNotEmpty()) {
            fail("適合しない(${report.failures.size} 件):\n" + report.failures.take(80).joinToString("\n"))
        }
    }

    private fun runCase(dir: File, report: Report) {
        val case = DslJson.parseToJsonElement(File(dir, "case.json").readText()).jsonObject
        val expected = DslJson.parseToJsonElement(File(dir, "expected.json").readText()).jsonObject
        val dslElement = case["dsl"]!!
        val dslJson = (dslElement as? JsonPrimitive)?.contentOrNull?.let { File(docsDir, it).readText() } ?: dslElement.toString()
        val dsl = AppDSL.decode(dslJson)

        val caps = case["device"]?.jsonObject?.get("capabilities") as? JsonObject
        val components = (caps?.get("components") as? JsonArray)?.map { it.jsonPrimitive.content }?.toSet() ?: allKnown
        val missing = (caps?.get("missingComponents") as? JsonArray)?.map { it.jsonPrimitive.content }?.toSet().orEmpty()
        val device = components - missing
        val tree = RenderTree(dsl, implemented intersect device)
        // 押せるかは iOS と同じ「全部品が登録済み」で判断する(Android にまだ無い button 以外の Leaf も押せる扱い)
        val tapTree = RenderTree(dsl, allKnown intersect device)

        val name = dir.name
        val expScreens = expected["screens"]!!.jsonArray
        val actScreens = dsl.screens.map { tree.screen(dsl.rootRouteFor(it.id)) }
        compareList(expScreens, actScreens, "$name:$.screens", report) { e, a, p -> compareScreen(e.jsonObject, a.jsonObject, p, report) }

        val ops = case["operations"] as? JsonArray ?: return
        val expSteps = expected["steps"]?.jsonArray ?: return
        var nav = NavigationState.initial(dsl)
        for ((i, op) in ops.withIndex()) {
            val path = "$name:$.steps[$i]"
            val next = applyNavOnly(op.jsonObject, nav, dsl, tapTree)
            if (next == null) {
                repeat(ops.size - i) { report.skip("step(フォーム・レコードの操作以降)") }
                return
            }
            val (state, ok) = next
            nav = state
            report.steps++
            val e = expSteps[i].jsonObject
            val top = nav.top(dsl)
            same(e["top"], RenderTree.route(top), "$path.top", report)
            same(e["error"] != null, !ok, "$path.error", report)
            e["screen"]?.let { compareScreen(it.jsonObject, tree.screen(top), "$path.screen", report) }
        }
    }

    /** 遷移だけの操作を適用する。フォーム・レコードに触れる操作なら null(以降は skip)。 */
    private fun applyNavOnly(op: JsonObject, nav: NavigationState, dsl: AppDSL, tapTree: RenderTree): Pair<NavigationState, Boolean>? {
        val route = nav.top(dsl)
        val screen = dsl.screen(route.screen)
        val action = when {
            op.containsKey("tap") -> {
                val id = op["tap"]!!.jsonPrimitive.content
                val c = screen?.components?.flatMap { listOf(it) + it.children }?.firstOrNull { it.id == id }
                if (c?.action == null || tapTree.state(c) != "rendered") return nav to false
                c.action!!
            }
            op.containsKey("tapToolbar") -> {
                val label = op["tapToolbar"]!!.jsonPrimitive.content
                screen?.toolbar?.firstOrNull { it.label == label }?.action ?: return nav to false
            }
            op.containsKey("back") -> return NavigationResolver.close(nav, route) to true
            else -> return null
        }
        return when (action.type) {
            "navigate" -> {
                if (action.prefillRef != null) return null
                val to = action.screen ?: return nav to true
                NavigationResolver.navigate(nav, to, Presentation.of(action.presentation), null, route, dsl) to true
            }
            "back" -> NavigationResolver.close(nav, route) to true
            "saveRecord", "deleteRecord", "resetForm", "follow", "unfollow" -> null
            // OS の機能に頼むもの(URL・通知・書き出し…)は遷移しない
            else -> nav to true
        }
    }

    private fun compareScreen(e: JsonObject, a: JsonObject, path: String, report: Report) {
        for (k in listOf("id", "title", "route", "layout", "missing")) same(e[k], a[k], "$path.$k", report)
        for (k in listOf("toolbar", "saveBar", "form")) if (e[k] != null) report.skip("screen.$k")
        compareList(e["nodes"]?.jsonArray, a["nodes"]?.jsonArray, "$path.nodes", report) { en, an, p -> compareNode(en.jsonObject, an.jsonObject, p, report) }
    }

    private fun compareNode(e: JsonObject, a: JsonObject, path: String, report: Report) {
        val type = e["type"]?.jsonPrimitive?.contentOrNull
        val state = e["state"]?.jsonPrimitive?.contentOrNull
        val isImplemented = type in implemented
        for (k in listOf("id", "cell", "stacked", "band", "localGrid")) same(e[k], a[k], "$path.$k", report)
        // 知らない type の文字列は :dsl の型が持っていない(生成器の型で持つ)
        if (state == "unknown") report.skip("unknown の type 文字列") else same(e["type"], a["type"], "$path.type", report)

        if (isImplemented || state == "unavailable" || state == "unknown") {
            same(e["state"], a["state"], "$path.state", report)
        } else {
            report.skip("Leaf の state($type)")
        }
        if (isImplemented) {
            same(e["props"], a["props"], "$path.props", report)
            same(e["actions"], a["actions"], "$path.actions", report)
        } else if (e["props"] != null || e["actions"] != null) {
            report.skip("Leaf の値($type)")
        }
        if (e["children"] != null || a["children"] != null) {
            compareList(e["children"]?.jsonArray, a["children"]?.jsonArray, "$path.children", report) { en, an, p ->
                compareNode(en.jsonObject, an.jsonObject, p, report)
            }
        }
    }

    private fun compareList(
        e: List<JsonElement>?,
        a: List<JsonElement>?,
        path: String,
        report: Report,
        each: (JsonElement, JsonElement, String) -> Unit,
    ) {
        val el = e.orEmpty()
        val al = a.orEmpty()
        if (el.size != al.size) {
            report.failures += "$path: 数が違う expected ${el.size} but got ${al.size}"
            return
        }
        el.indices.forEach { each(el[it], al[it], "$path[$it]") }
    }

    private fun same(e: Any?, a: Any?, path: String, report: Report) {
        val en = normalize(e)
        val an = normalize(a)
        if (en != an) report.failures += "$path: expected ${short(e)} but got ${short(a)}"
    }

    /** JSON として同じか(キーの順・1200 と 1200.0 は見ない)。 */
    private fun normalize(v: Any?): Any? = when (v) {
        null, JsonNull -> null
        is JsonObject -> v.mapValues { normalize(it.value) }.filterValues { it != null }
        is JsonArray -> v.map { normalize(it) }
        is JsonPrimitive -> if (v.isString) v.content else v.booleanOrNull ?: v.doubleOrNull ?: v.content
        else -> v
    }

    private fun short(v: Any?): String = v.toString().let { if (it.length > 160) it.take(160) + "…" else it }
}
