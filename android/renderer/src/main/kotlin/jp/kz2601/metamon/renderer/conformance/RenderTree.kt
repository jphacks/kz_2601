package jp.kz2601.metamon.renderer.conformance

import jp.kz2601.metamon.dsl.Action
import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.dsl.screen
import jp.kz2601.metamon.dsl.records.FieldCoding
import jp.kz2601.metamon.renderer.components.lists.ChartBuckets
import jp.kz2601.metamon.renderer.components.lists.GaugeValue
import jp.kz2601.metamon.renderer.components.lists.LIST_EMPTY_TEXT
import jp.kz2601.metamon.renderer.components.lists.LeafData
import jp.kz2601.metamon.renderer.components.lists.ListRows
import jp.kz2601.metamon.renderer.components.lists.ListToggle
import jp.kz2601.metamon.renderer.components.lists.ProgressValue
import jp.kz2601.metamon.renderer.components.lists.StatValue
import jp.kz2601.metamon.renderer.components.lists.TableCells
import jp.kz2601.metamon.renderer.components.lists.TimelineRows
import jp.kz2601.metamon.renderer.DSLRuntime
import jp.kz2601.metamon.renderer.NoopDSLRuntime
import jp.kz2601.metamon.renderer.components.disclosureTitle
import jp.kz2601.metamon.renderer.components.inputs.InputRules
import jp.kz2601.metamon.renderer.form.FormDraft
import jp.kz2601.metamon.renderer.components.gridColumns
import jp.kz2601.metamon.renderer.components.isButtonVisible
import jp.kz2601.metamon.renderer.components.linkOpens
import jp.kz2601.metamon.renderer.components.linkText
import jp.kz2601.metamon.renderer.components.photoFieldValue
import jp.kz2601.metamon.renderer.layout.GridArea
import jp.kz2601.metamon.renderer.layout.formBands
import jp.kz2601.metamon.renderer.layout.gridPlacement
import jp.kz2601.metamon.renderer.layout.isFormLayout
import jp.kz2601.metamon.renderer.layout.isVoiceScreen
import jp.kz2601.metamon.renderer.layout.localGridPlacement
import jp.kz2601.metamon.renderer.navigation.Route
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.doubleOrNull

/**
 * 適合テスト(#352)の「描画の木」の Android 版。画面を描かずに DSL → 何が・どこに・押すと何が起きるか を JSON にする。
 * 形は docs/dsl/conformance/README.md、iOS の Conformance/RenderTree.swift と同じ。配置は View と同じ関数
 * (`gridPlacement` / `localGridPlacement` / `formBands`)から取る。
 *
 * まだ入れていないもの(子 Issue): toolbar / saveBar・text の `{{field}}` の差し込み。
 * 一覧・集計・グラフ(#859)の値は各 Leaf と同じ関数(components/lists/ListValues.kt)から取る。レジストリに無い Leaf は `unavailable` になる。
 *
 * @param available 描ける type(レジストリの登録 ∩ 端末の能力)。外れた既知の type は `unavailable`、知らない type は `unknown`。
 */
class RenderTree(
    private val dsl: AppDSL,
    private val available: Set<String>,
    /** 記録・entity・サインイン中の uid(フォームと入力部品が読む) */
    private val runtime: DSLRuntime = NoopDSLRuntime,
) {

    /** route の画面 1 枚。[form] は `ScreenForm.makeDraft` で作ったもの(フォーム画面でなければ null)。 */
    fun screen(route: Route, form: FormDraft? = null): JsonObject {
        val screen = dsl.screen(route.screen)
            ?: return obj("route" to route(route), "missing" to JsonPrimitive(true))
        val placement = screen.gridPlacement
        val isForm = screen.isFormLayout
        val bandOf = java.util.IdentityHashMap<Component, Int>()
        if (isForm) {
            formBands(placement.placed).forEachIndexed { i, band -> band.items.forEach { bandOf[it.component] = i } }
        }
        val nodes = placement.placedInReadingOrder.map { (c, a) ->
            node(c, a, bandOf[c]?.let { mapOf("band" to JsonPrimitive(it)) }.orEmpty(), form)
        } + placement.stacked.map { node(it, null, mapOf("stacked" to JsonPrimitive(true)), form) }

        val out = obj(
            "id" to JsonPrimitive(screen.id),
            "title" to JsonPrimitive(screen.title),
            "route" to route(route),
            "layout" to JsonPrimitive(if (screen.isVoiceScreen) "voice" else if (isForm) "form" else "grid"),
            "nodes" to JsonArray(nodes),
        )
        return if (form == null) out else JsonObject(out + ("form" to InputNodes.form(form)))
    }

    fun node(c: Component, cell: GridArea?, extra: Map<String, JsonElement> = emptyMap(), form: FormDraft? = null): JsonObject {
        val out = LinkedHashMap(extra)
        out["id"] = c.id?.let(::JsonPrimitive) ?: JsonNull
        out["type"] = JsonPrimitive(c.type.wire)
        cell?.let { out["cell"] = cell(it) }
        val state = state(c, form)
        out["state"] = JsonPrimitive(state)
        if (state != "rendered") return JsonObject(out)

        val props = declared(c) + resolved(c) + InputNodes.props(c, form, runtime) + PlayNodes.props(c, runtime)
        if (props.isNotEmpty()) out["props"] = JsonObject(props)
        val actions = actions(c)
        if (actions.isNotEmpty()) out["actions"] = JsonArray(actions)
        if (c.type.isContainer) {
            val local = c.localGridPlacement
            if (local != null) {
                out["localGrid"] = JsonPrimitive(true)
                out["children"] = JsonArray(local.map { node(it.component, it.area, form = form) })
            } else {
                out["children"] = JsonArray(c.children.map { node(it, null, form = form) })
            }
        }
        return JsonObject(out)
    }

    /** rendered / hidden / unavailable / unknown(`DSLComponent` と各 Leaf の分岐と同じ順)。 */
    fun state(c: Component, form: FormDraft? = null): String {
        InputNodes.state(c, form, runtime)?.let { return it }
        if (c.type.wire !in available) return if (c.type == ComponentType.UNKNOWN) "unknown" else "unavailable"
        if (c.type == ComponentType.BUTTON && !isButtonVisible(c.action, form)) return "hidden"
        if (c.type in InputRules.inputTypes && (form == null || c.field == null)) return "needsForm"
        return "rendered"
    }

    /** DSL に書いてあり OS が変わっても意味が同じ値(:dsl の型が持っているものだけ)。 */
    private fun declared(c: Component): Map<String, JsonElement> = buildMap {
        c.title?.let { put("title", JsonPrimitive(it)) }
        c.text?.let { put("text", JsonPrimitive(it)) }
        c.label?.let { put("label", JsonPrimitive(it)) }
        c.message?.let { put("message", JsonPrimitive(it)) }
        c.url?.let { put("url", JsonPrimitive(it)) }
        c.placeholder?.let { put("placeholder", JsonPrimitive(it)) }
        c.field?.let { put("field", JsonPrimitive(it)) }
        c.entity?.let { put("entity", JsonPrimitive(it)) }
        c.aggregate?.let { put("aggregate", JsonPrimitive(it.wire)) }
        c.format?.let { put("format", JsonPrimitive(it.wire)) }
        c.groupBy?.let { put("groupBy", JsonPrimitive(it)) }
        c.bucket?.let { put("bucket", JsonPrimitive(it.wire)) }
        c.kind?.let { put("kind", JsonPrimitive(it.wire)) }
        c.unit?.let { put("unit", JsonPrimitive(it)) }
        c.min?.let { put("min", JsonPrimitive(it)) }
        c.max?.let { put("max", JsonPrimitive(it)) }
        c.step?.let { put("step", JsonPrimitive(it)) }
        // target は数のリテラルだけ(式の目標は resolved の target)
        (c.target as? JsonPrimitive)?.takeIf { !it.isString && it.doubleOrNull != null }?.let { put("target", it) }
        c.limit?.let { put("limit", JsonPrimitive(it)) }
        c.expanded?.let { put("expanded", JsonPrimitive(it)) }
    }

    /** 解決した値。View と同じ関数を使う。 */
    private fun resolved(c: Component): Map<String, JsonElement> = when (c.type) {
        ComponentType.DISCLOSURE -> mapOf(
            "title" to JsonPrimitive(c.disclosureTitle),
            "expanded" to JsonPrimitive(c.expanded ?: false),
        )
        ComponentType.GRID -> mapOf("columns" to JsonPrimitive(c.gridColumns))
        ComponentType.LINK -> mapOf("text" to JsonPrimitive(c.linkText()), "opens" to JsonPrimitive(c.linkOpens))
        // フォームのドラフトが入るまで field の値は無い(photoFieldValue)
        ComponentType.IMAGE -> mapOf("hasImage" to JsonPrimitive(photoFieldValue(c, null) != null))
        ComponentType.STAT -> StatValue.resolve(c, leafData()).let {
            mapOf("value" to JsonPrimitive(it.text), "unavailable" to JsonPrimitive(it.unavailable))
        }
        ComponentType.LIST -> listProps(c, leafData())
        ComponentType.TABLE -> {
            val data = leafData()
            val entity = data.entity(c.entity)
            mapOf(
                "columns" to JsonArray(c.columns.map { JsonPrimitive(TableCells.header(it, entity)) }),
                "rows" to JsonArray(TableCells.rows(c, data).map { r -> JsonArray(r.texts.map(::JsonPrimitive)) }),
            )
        }
        ComponentType.TIMELINE -> {
            val data = leafData()
            val entity = data.entity(c.entity)
            mapOf(
                "rows" to JsonArray(
                    TimelineRows.rows(c, data).map { r ->
                        JsonObject(
                            buildMap {
                                put("recordId", JsonPrimitive(r.id))
                                put("date", JsonPrimitive(data.text(r, c.dateField ?: "", entity)))
                                put("title", JsonPrimitive(data.text(r, c.titleField ?: "", entity)))
                                c.subtitleField?.let { data.text(r, it, entity) }?.takeIf { it.isNotEmpty() }?.let { put("subtitle", JsonPrimitive(it)) }
                            },
                        )
                    },
                ),
            )
        }
        ComponentType.CHART -> mapOf(
            "buckets" to JsonArray(
                ChartBuckets.buckets(c, leafData()).map { JsonObject(mapOf("label" to JsonPrimitive(it.label), "value" to JsonPrimitive(it.value))) },
            ),
        )
        ComponentType.PROGRESS_BAR -> {
            val data = leafData()
            val raw = ProgressValue.ratio(c, data)
            val percent = if (ProgressValue.missingTarget(c, data)) FieldCoding.UNAVAILABLE else ProgressValue.percentText(raw, c.tone?.wire)
            buildMap {
                put("ratio", JsonPrimitive(raw))
                put("percent", JsonPrimitive(percent))
                if (ProgressValue.targetExpr(c) != null) ProgressValue.target(c, data)?.let { put("target", JsonPrimitive(it)) }
            }
        }
        ComponentType.GAUGE -> {
            val (value, unit) = GaugeValue.value(c, leafData())
            buildMap {
                put("value", JsonPrimitive(value))
                unit?.let { put("unit", JsonPrimitive(it)) }
            }
        }
        // 画面を描かないので検索文字列は空(描く時は ScreenLocals)
        ComponentType.SEARCH_BAR -> mapOf("query" to JsonPrimitive(""))
        // aiButton: DSL に書いた値だけ(結果は AI 次第なので木に入れない)
        ComponentType.AI_BUTTON -> buildMap {
            c.task?.let { put("task", JsonPrimitive(it.wire)) }
            c.output?.let { put("output", JsonPrimitive(it)) }
            c.entity?.let { put("entity", JsonPrimitive(it)) }
            c.limit?.let { put("limit", JsonPrimitive(it)) }
        }
        else -> emptyMap()
    }

    private fun leafData() = LeafData(runtime, runtime.recordsFlow?.value.orEmpty())

    private fun listProps(c: Component, data: LeafData): Map<String, JsonElement> {
        val entity = data.entity(c.entity)
        val rows = ListRows.rows(c, data, null)
        val toggleField = ListToggle.field(c.item, entity)
        val out = linkedMapOf<String, JsonElement>(
            "rows" to JsonArray(
                rows.map { r ->
                    val t = ListRows.texts(c.item, r, entity, data)
                    JsonObject(
                        buildMap {
                            put("recordId", JsonPrimitive(r.id))
                            put("title", JsonPrimitive(t.title))
                            t.subtitle?.let { put("subtitle", JsonPrimitive(it)) }
                            t.trailing?.let { put("trailing", JsonPrimitive(it)) }
                            toggleField?.let { f -> put("checked", JsonPrimitive(ListToggle.isOn(r, f.id, ListToggle.isDaily(c.item), emptyMap(), data.now))) }
                        },
                    )
                },
            ),
        )
        if (rows.isEmpty()) out["emptyText"] = JsonPrimitive(c.emptyText ?: LIST_EMPTY_TEXT)
        return out
    }

    private fun actions(c: Component): List<JsonElement> = listOfNotNull(
        c.action?.let { obj("on" to JsonPrimitive("tap"), "action" to action(it)) },
        c.onItemTap?.let { obj("on" to JsonPrimitive("rowTap"), "action" to action(it)) },
        c.onDateTap?.let { obj("on" to JsonPrimitive("dateTap"), "action" to action(it)) },
    )

    companion object {
        fun route(r: Route): JsonObject = obj(
            "screen" to JsonPrimitive(r.screen),
            "stack" to JsonPrimitive(r.stack),
            "recordId" to (r.recordId?.let(::JsonPrimitive) ?: JsonNull),
        )

        /** 0 始まりの行・列と幅(GridArea は行が 1 始まりなので 1 引く)。 */
        fun cell(a: GridArea): JsonObject = obj(
            "row" to JsonPrimitive(a.row1 - 1),
            "col" to JsonPrimitive(a.col1),
            "rowSpan" to JsonPrimitive(a.rowSpan),
            "colSpan" to JsonPrimitive(a.colSpan),
        )

        fun action(a: Action): JsonObject = JsonObject(
            buildMap {
                put("type", JsonPrimitive(a.type.wire))
                a.screen?.let { put("screen", JsonPrimitive(it)) }
                a.presentation?.let { put("presentation", JsonPrimitive(it.wire)) }
                a.prefillRef?.let { put("prefillRef", JsonPrimitive(it)) }
                a.entity?.let { put("entity", JsonPrimitive(it)) }
                a.url?.let { put("url", JsonPrimitive(it)) }
                a.confirmMessage?.let { put("confirmMessage", JsonPrimitive(it)) }
                a.format?.let { put("format", JsonPrimitive(it.wire)) }
                a.notificationId?.let { put("notificationId", JsonPrimitive(it)) }
            },
        )

        private fun obj(vararg pairs: Pair<String, JsonElement>) = JsonObject(mapOf(*pairs))
    }
}
