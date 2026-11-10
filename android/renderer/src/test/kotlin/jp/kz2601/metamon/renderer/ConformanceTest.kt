package jp.kz2601.metamon.renderer

import jp.kz2601.metamon.dsl.ActionType
import jp.kz2601.metamon.dsl.Action
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.Screen
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.renderer.components.lists.LeafData
import jp.kz2601.metamon.renderer.components.lists.ListRows
import jp.kz2601.metamon.renderer.components.lists.ScreenLocals
import jp.kz2601.metamon.renderer.form.FormDraft
import jp.kz2601.metamon.renderer.form.FormPrefill
import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.decode
import jp.kz2601.metamon.dsl.screen
import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.dsl.DslJson
import jp.kz2601.metamon.dsl.expr.DSLClock
import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.FieldCoding
import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.dsl.records.RecordPermission
import jp.kz2601.metamon.dsl.records.entityModels
import jp.kz2601.metamon.renderer.conformance.RenderTree
import jp.kz2601.metamon.renderer.form.ScreenForm
import jp.kz2601.metamon.renderer.navigation.Route
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow
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
import java.time.OffsetDateTime
import java.time.ZoneId
import java.util.Locale

/**
 * 適合テスト(#352)を Android で回す。docs/dsl/conformance/cases の case.json を読み、[RenderTree] の木を
 * iOS が書いた expected.json と比べる(expected.json は書き換えない)。
 *
 * #357 で比べるもの: 画面の id・title・route・layout、部品の id・type・cell・stacked・band・localGrid・children、
 * Container 7 種と Android に登録済みの Leaf の state・props・actions、遷移だけの操作(tap / tapToolbar / back)の後の top と画面。
 * #858 で足したもの: フォーム画面の `form`(下書きの値)と入力 12 種の state(hidden / needsForm)・props(束縛中の値)。
 * #859 で足したもの: 一覧・集計・グラフ 10 種の props(記録から解決した行・値・棒)と rowTap / dateTap の actions。
 * #851 で足したもの: 画面の toolbar・saveBar、知らない type の文字列、全部の操作(input・search・tapRow・保存・削除・
 * prefillRef)とその後の top・画面・effects。操作は [CaseSession](iOS の ConformanceRuntime と同じ規則)が実行する。
 *
 * skip(数だけ出して赤にしない): Android にまだ無い Leaf の state・props・actions(`unavailable` / `unknown` の state は比べる)。
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
        val device = case["device"] as? JsonObject
        val saved = Triple(DSLClock.fixed, FieldCoding.zone, Locale.getDefault())
        try {
            DSLClock.fixed = OffsetDateTime.parse(device?.str("now") ?: "2026-11-12T12:00:00+09:00").toInstant()
            FieldCoding.zone = ZoneId.of(device?.str("timeZone") ?: "Asia/Tokyo")
            Locale.setDefault(Locale.forLanguageTag((device?.str("locale") ?: "ja_JP").replace('_', '-')))
            runCase(dir, case, report)
        } finally {
            DSLClock.fixed = saved.first
            FieldCoding.zone = saved.second
            Locale.setDefault(saved.third)
        }
    }

    private fun JsonObject.str(key: String): String? = (this[key] as? JsonPrimitive)?.contentOrNull

    private fun runCase(dir: File, case: JsonObject, report: Report) {
        val expected = DslJson.parseToJsonElement(File(dir, "expected.json").readText()).jsonObject
        val dslElement = case["dsl"]!!
        val dslJson = (dslElement as? JsonPrimitive)?.contentOrNull?.let { File(docsDir, it).readText() } ?: dslElement.toString()
        val dsl = AppDSL.decode(dslJson)

        val caps = case["device"]?.jsonObject?.get("capabilities") as? JsonObject
        val components = (caps?.get("components") as? JsonArray)?.map { it.jsonPrimitive.content }?.toSet() ?: allKnown
        val missing = (caps?.get("missingComponents") as? JsonArray)?.map { it.jsonPrimitive.content }?.toSet().orEmpty()
        val device = components - missing
        val records = (case["records"] as? JsonObject).orEmpty().mapValues { (_, v) -> v.jsonArray.map { Record.from(it) } }
        val runtime = CaseRuntime(dsl, records, (case["device"] as? JsonObject)?.str("userId"))
        val tree = RenderTree(dsl, implemented intersect device, runtime)
        // 押せるかは iOS と同じ「全部品が登録済み」で判断する(Android にまだ無い button 以外の Leaf も押せる扱い)
        val tapTree = RenderTree(dsl, allKnown intersect device, runtime)
        val session = CaseSession(dsl, runtime, tapTree)
        fun screenOf(route: Route): JsonObject {
            val (form, locals) = session.session(route)
            return tree.screen(route, form, locals)
        }

        val name = dir.name
        val expScreens = expected["screens"]!!.jsonArray
        val actScreens = dsl.screens.map { screenOf(dsl.rootRouteFor(it.id)) }
        compareList(expScreens, actScreens, "$name:$.screens", report) { e, a, p -> compareScreen(e.jsonObject, a.jsonObject, p, report) }

        val ops = case["operations"] as? JsonArray ?: return
        val expSteps = expected["steps"]?.jsonArray ?: return
        if (expSteps.size != ops.size) report.failures += "$name:$.steps: 数が違う expected ${expSteps.size} but got ${ops.size}"
        for ((i, op) in ops.withIndex()) {
            val path = "$name:$.steps[$i]"
            session.effects.clear()
            val ok = session.apply(op.jsonObject)
            report.steps++
            val e = expSteps.getOrNull(i)?.jsonObject ?: break
            val top = session.top
            same(e["top"], RenderTree.route(top), "$path.top", report)
            same(e["error"] != null, !ok, "$path.error", report)
            same(e["effects"] ?: JsonArray(emptyList()), JsonArray(session.effects), "$path.effects", report)
            e["screen"]?.let { compareScreen(it.jsonObject, screenOf(top), "$path.screen", report) }
        }
    }

    private fun compareScreen(e: JsonObject, a: JsonObject, path: String, report: Report) {
        for (k in listOf("id", "title", "route", "layout", "missing", "form", "toolbar", "saveBar")) same(e[k], a[k], "$path.$k", report)
        compareList(e["nodes"]?.jsonArray, a["nodes"]?.jsonArray, "$path.nodes", report) { en, an, p -> compareNode(en.jsonObject, an.jsonObject, p, report) }
    }

    private fun compareNode(e: JsonObject, a: JsonObject, path: String, report: Report) {
        val type = e["type"]?.jsonPrimitive?.contentOrNull
        val state = e["state"]?.jsonPrimitive?.contentOrNull
        val isImplemented = type in implemented
        for (k in listOf("id", "type", "cell", "stacked", "band", "localGrid")) same(e[k], a[k], "$path.$k", report)

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

/** case の記録と uid を持つ能力(iOS の ConformanceRuntime の読む側)。保存・削除で記録が変わる。 */
private class CaseRuntime(dsl: AppDSL, records: Map<String, List<Record>>, private val uid: String?) : DSLRuntime {
    override val entityModels: List<Entity> = dsl.entityModels()
    private val store = MutableStateFlow(records)
    override val recordsFlow: StateFlow<Map<String, List<Record>>> = store

    fun records(entity: String): List<Record> = store.value[entity].orEmpty()

    fun put(entity: String, rows: List<Record>) {
        store.value = store.value + (entity to rows)
    }

    override fun onVoiceInput(componentId: String?) = Unit

    override fun canEdit(entity: String, record: Record): Boolean = RecordPermission.canEdit(record, entityModel(entity), uid)

    override fun canDelete(entity: String, record: Record): Boolean = RecordPermission.canDelete(record, entityModel(entity), uid)

    override fun currentUid(): String? = uid
}

/**
 * case の操作を実行する(iOS の ConformanceRuntime の `apply` / `perform`。アプリ本体の保存・削除の規則を写したもの)。
 * 遷移はアプリと同じ [NavigationResolver]。フォームの下書きと検索文字列は route ごとに 1 つ持つ(画面の状態に当たる)。
 */
private class CaseSession(private val dsl: AppDSL, private val runtime: CaseRuntime, private val tapTree: RenderTree) {
    private var nav = NavigationState.initial(dsl)
    private val sessions = HashMap<String, Pair<FormDraft?, ScreenLocals>>()
    private var nextId = 1
    val effects = mutableListOf<JsonElement>()

    val top: Route get() = nav.top(dsl)

    private val Screen.allComponents: List<Component> get() = components.flatMap { listOf(it) + it.children }

    fun session(route: Route): Pair<FormDraft?, ScreenLocals> = sessions.getOrPut(route.id) {
        val entity = dsl.screen(route.screen)?.entity?.let(runtime::entityModel)
        val form = entity?.let { ScreenForm.makeDraft(it, ScreenForm.formRecord(it, route.recordId, runtime.records(it.id)), runtime) }
        form to ScreenLocals()
    }

    /** 操作 1 つ。対象が無い・見えていないなら false。 */
    fun apply(op: JsonObject): Boolean {
        val route = top
        val (form, locals) = session(route)
        val screen = dsl.screen(route.screen)
        val ctx = ActionContext(route, form)
        when {
            op.containsKey("tap") -> {
                val id = op["tap"]!!.jsonPrimitive.content
                val c = screen?.allComponents?.firstOrNull { it.id == id } ?: return false
                val a = c.action ?: return false
                if (tapTree.state(c, form) != "rendered") return false
                dispatch(a, ctx)
            }
            op.containsKey("tapToolbar") -> {
                val label = op["tapToolbar"]!!.jsonPrimitive.content
                val tb = screen?.toolbar?.firstOrNull { it.label == label } ?: return false
                dispatch(tb.action, ctx)
            }
            op.containsKey("tapRow") -> {
                val row = op["tapRow"]!!.jsonObject
                // component 省略時は画面で最初の onItemTap を持つ部品(id の無い list 用)
                val id = (row["component"] as? JsonPrimitive)?.contentOrNull
                val c = screen?.allComponents?.firstOrNull { if (id == null) it.onItemTap != null else it.id == id } ?: return false
                val a = c.onItemTap ?: return false
                if (tapTree.state(c, form) != "rendered") return false
                val query = c.searchField?.let { locals.values[it] }
                val rows = ListRows.rows(c, LeafData(runtime, runtime.recordsFlow.value), query)
                val i = (row["index"] as? JsonPrimitive)?.doubleOrNull?.toInt() ?: 0
                val r = rows.getOrNull(i) ?: return false
                dispatch(a, ActionContext(route, form, r.id))
            }
            op.containsKey("input") -> {
                val input = op["input"]!!.jsonObject
                val f = (input["field"] as? JsonPrimitive)?.contentOrNull
                if (form == null || f == null) return false
                form.values[f] = JSONValue.from(input["value"] ?: JsonNull)
            }
            op.containsKey("search") -> {
                val search = op["search"]!!.jsonObject
                val key = (search["field"] as? JsonPrimitive)?.contentOrNull ?: return false
                locals.values[key] = (search["text"] as? JsonPrimitive)?.contentOrNull.orEmpty()
            }
            op.containsKey("back") -> nav = NavigationResolver.close(nav, route)
            else -> return false
        }
        return true
    }

    private fun dispatch(action: Action, ctx: ActionContext) {
        // 確認は「はい」で進める
        action.confirmMessage?.let { effects += obj("confirm" to JsonPrimitive(it)) }
        perform(action, ctx)
    }

    private fun navigate(to: String, action: Action, recordId: String?, from: Route) {
        nav = NavigationResolver.navigate(nav, to, Presentation.of(action.presentation?.wire), recordId, from, dsl)
    }

    private fun perform(action: Action, ctx: ActionContext) {
        when (action.type) {
            ActionType.NAVIGATE -> {
                val to = action.screen ?: return
                val ref = action.prefillRef
                if (ref == null) {
                    navigate(to, action, ctx.recordId, ctx.route)
                    return
                }
                val parentId = ctx.currentRecordId
                if (parentId == null) {
                    effects += obj("rejected" to JsonPrimitive("unsavedParent"))
                    return
                }
                FormPrefill.set(mapOf(ref to JSONValue.Str(parentId)))
                navigate(to, action, null, ctx.route)
                session(top) // 新規フォームは開いた時に初期値を受け取る
            }
            ActionType.BACK -> nav = NavigationResolver.close(nav, ctx.route)
            ActionType.SAVE_RECORD -> save(action, ctx)
            ActionType.DELETE_RECORD -> {
                val entity = action.entity ?: ctx.form?.entity?.id ?: return
                val id = ctx.recordId ?: ctx.form?.recordId ?: return
                runtime.put(entity, runtime.records(entity).filter { it.id != id })
                effects += obj("deleted" to obj("entity" to JsonPrimitive(entity), "recordId" to JsonPrimitive(id)))
                if (ctx.recordId == null) nav = NavigationResolver.close(nav, ctx.route)
            }
            // OS の機能を呼ぶもの(URL・通知・書き出し・ライブアクティビティ・音声)は「何を頼んだか」だけ残す
            else -> effects += obj("request" to RenderTree.action(action))
        }
    }

    private fun save(action: Action, ctx: ActionContext) {
        val form = ctx.form
        val entity = action.entity ?: form?.entity?.id
        if (form == null || entity == null) {
            effects += obj("rejected" to JsonPrimitive("noForm"))
            return
        }
        val missing = form.missingRequiredFields.map { it.name }
        if (missing.isNotEmpty()) {
            effects += obj("rejected" to obj("missing" to JsonArray(missing.map(::JsonPrimitive))))
            return
        }
        val payload = form.payload
        val rows = runtime.records(entity)
        val existing = form.recordId?.takeIf { id -> rows.any { it.id == id } }
        val id = existing ?: "new-${nextId++}"
        runtime.put(entity, if (existing != null) rows.map { if (it.id == id) it.copy(data = payload) else it } else rows + Record(id, payload))
        effects += obj(
            "saved" to obj(
                "entity" to JsonPrimitive(entity),
                "recordId" to JsonPrimitive(id),
                "data" to JsonObject(payload.mapValues { it.value.toJsonElement() }),
            ),
        )
        sessions.entries.removeAll { it.value.first === form } // 保存した画面のドラフトは捨てる(開き直すと新規)
        val to = action.screen
        if (to != null) navigate(to, action, null, ctx.route) else nav = NavigationResolver.close(nav, ctx.route)
    }

    private fun obj(vararg pairs: Pair<String, JsonElement>) = JsonObject(mapOf(*pairs))
}
