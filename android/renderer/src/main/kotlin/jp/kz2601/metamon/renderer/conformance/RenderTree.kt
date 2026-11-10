package jp.kz2601.metamon.renderer.conformance

import jp.kz2601.metamon.dsl.Action
import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.dsl.screen
import jp.kz2601.metamon.renderer.components.disclosureTitle
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

/**
 * 適合テスト(#352)の「描画の木」の Android 版。画面を描かずに DSL → 何が・どこに・押すと何が起きるか を JSON にする。
 * 形は docs/dsl/conformance/README.md、iOS の Conformance/RenderTree.swift と同じ。配置は View と同じ関数
 * (`gridPlacement` / `localGridPlacement` / `formBands`)から取る。
 *
 * まだ入れていないもの(子 Issue): フォームのドラフト(`form`・`hidden`・`needsForm`)・toolbar / saveBar・
 * レコードから解決する値(stat / list / chart …)・text の `{{field}}` の差し込み。レジストリに無い Leaf は `unavailable` になる。
 *
 * @param available 描ける type(レジストリの登録 ∩ 端末の能力)。外れた既知の type は `unavailable`、知らない type は `unknown`。
 */
class RenderTree(private val dsl: AppDSL, private val available: Set<String>) {

    /** route の画面 1 枚。 */
    fun screen(route: Route): JsonObject {
        val screen = dsl.screen(route.screen)
            ?: return obj("route" to route(route), "missing" to JsonPrimitive(true))
        val placement = screen.gridPlacement
        val isForm = screen.isFormLayout
        val bandOf = java.util.IdentityHashMap<Component, Int>()
        if (isForm) {
            formBands(placement.placed).forEachIndexed { i, band -> band.items.forEach { bandOf[it.component] = i } }
        }
        val nodes = placement.placedInReadingOrder.map { (c, a) ->
            node(c, a, bandOf[c]?.let { mapOf("band" to JsonPrimitive(it)) }.orEmpty())
        } + placement.stacked.map { node(it, null, mapOf("stacked" to JsonPrimitive(true))) }

        return obj(
            "id" to JsonPrimitive(screen.id),
            "title" to JsonPrimitive(screen.title),
            "route" to route(route),
            "layout" to JsonPrimitive(if (screen.isVoiceScreen) "voice" else if (isForm) "form" else "grid"),
            "nodes" to JsonArray(nodes),
        )
    }

    fun node(c: Component, cell: GridArea?, extra: Map<String, JsonElement> = emptyMap()): JsonObject {
        val out = LinkedHashMap(extra)
        out["id"] = c.id?.let(::JsonPrimitive) ?: JsonNull
        out["type"] = JsonPrimitive(c.type.wire)
        cell?.let { out["cell"] = cell(it) }
        val state = state(c)
        out["state"] = JsonPrimitive(state)
        if (state != "rendered") return JsonObject(out)

        val props = declared(c) + resolved(c)
        if (props.isNotEmpty()) out["props"] = JsonObject(props)
        val actions = actions(c)
        if (actions.isNotEmpty()) out["actions"] = JsonArray(actions)
        if (c.type.isContainer) {
            val local = c.localGridPlacement
            if (local != null) {
                out["localGrid"] = JsonPrimitive(true)
                out["children"] = JsonArray(local.map { node(it.component, it.area) })
            } else {
                out["children"] = JsonArray(c.children.map { node(it, null) })
            }
        }
        return JsonObject(out)
    }

    /** rendered / hidden / unavailable / unknown(`DSLComponent` と各 Leaf の分岐と同じ順)。 */
    fun state(c: Component): String {
        if (c.type.wire !in available) return if (c.type == ComponentType.UNKNOWN) "unknown" else "unavailable"
        if (c.type == ComponentType.BUTTON && !isButtonVisible(c.action)) return "hidden"
        return "rendered"
    }

    /** DSL に書いてあり OS が変わっても意味が同じ値(:dsl の型が持っているものだけ)。 */
    private fun declared(c: Component): Map<String, JsonElement> = buildMap {
        c.title?.let { put("title", JsonPrimitive(it)) }
        c.text?.let { put("text", JsonPrimitive(it)) }
        c.label?.let { put("label", JsonPrimitive(it)) }
        c.message?.let { put("message", JsonPrimitive(it)) }
        c.url?.let { put("url", JsonPrimitive(it)) }
        c.field?.let { put("field", JsonPrimitive(it)) }
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
        else -> emptyMap()
    }

    private fun actions(c: Component): List<JsonElement> =
        listOfNotNull(c.action?.let { obj("on" to JsonPrimitive("tap"), "action" to action(it)) })

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
