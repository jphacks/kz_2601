package jp.kz2601.metamon.desktop

import jp.kz2601.metamon.desktop.io.IoLeaves
import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.dsl.decode
import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.renderer.form.FormDraft
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.jsonArray
import kotlinx.serialization.json.jsonObject

/** デスクトップ版で描ける部品(骨格の段階)。ここに無い既知の type は「この端末では使えない」(unavailable)として穴を残す。 */
object DesktopLeaves {
    val containers: Set<ComponentType> = ComponentType.entries.filter { it.isContainer }.toSet()

    val leaves: Set<ComponentType> = setOf(
        ComponentType.TEXT,
        ComponentType.LABEL,
        ComponentType.MARKDOWN,
        ComponentType.BADGE,
        ComponentType.EMPTY_STATE,
        ComponentType.SPACER,
        ComponentType.DIVIDER,
        ComponentType.BUTTON,
        ComponentType.LINK,
        ComponentType.LIST,
        ComponentType.TEXT_FIELD,
        ComponentType.NUMBER_FIELD,
        ComponentType.TEXT_EDITOR,
        ComponentType.SECURE_FIELD,
        ComponentType.TOGGLE,
        ComponentType.PICKER,
        ComponentType.STEPPER,
        ComponentType.OPEN_URL_BUTTON,
        *PlayLeaves.types.toTypedArray(),
    ) + DISPLAY_LEAVES + InputLeaves.types + DATA_LEAVES

    /**
     * 部品の state: rendered / hidden / needsForm / unavailable / unknown(iOS の RenderTree.state と同じ順)。
     * 描画・操作(押せるか)・適合テストの木はすべてこれを見る。[form] は開いている画面のフォーム(無ければ null)。
     */
    fun state(c: Component, form: FormDraft? = null, uid: String? = null): String {
        if (form != null && form.hides(c, uid)) return "hidden"
        if (c.type !in containers && c.type !in leaves && c.type !in IoLeaves.types) {
            return if (c.type == ComponentType.UNKNOWN) "unknown" else "unavailable"
        }
        if (c.type == ComponentType.BUTTON && !isButtonVisible(c.action, form)) return "hidden"
        if (InputLeaves.isInput(c) && (form == null || c.field == null)) return "needsForm"
        return "rendered"
    }

    /** DSL に書いてある type の文字列そのまま(知らない type も)。 */
    fun typeName(c: Component): String = c.rawType.ifEmpty { c.type.wire }
}

/** アプリに同梱した例の DSL(docs/dsl/examples)。 */
object Examples {
    fun load(name: String): AppDSL {
        val path = "examples/${name.removeSuffix(".json")}.json"
        val text = Examples::class.java.classLoader.getResourceAsStream(path)?.use { it.readBytes().decodeToString() }
            ?: error("同梱の DSL が無い: $path")
        return AppDSL.decode(text)
    }
}

/** モックのレコード(サインインも通信もしない骨格用)。 */
/** createdAt / author は読み込んだ記録のもの(無ければ並び順・自分)。createdAt の "" は「分からない」(並べ替えで元の順を保つ)。 */
data class Row(val id: String, val data: Map<String, String>, val createdAt: String? = null, val author: String? = null)

data class MockStore(val rows: Map<String, List<Row>> = emptyMap(), val nextId: Int = 1) {
    fun list(entity: String): List<Row> = rows[entity].orEmpty()

    fun find(entity: String, id: String): Row? = list(entity).firstOrNull { it.id == id }

    /** [newId] は新規の時の id(無ければ new-N)。 */
    fun save(entity: String, id: String?, data: Map<String, String>, newId: String? = null): MockStore {
        val current = list(entity)
        return if (id != null && current.any { it.id == id }) {
            copy(rows = rows + (entity to current.map { if (it.id == id) it.copy(data = data) else it }))
        } else {
            copy(rows = rows + (entity to current + Row(newId ?: "new-$nextId", data)), nextId = nextId + 1)
        }
    }

    fun delete(entity: String, id: String): MockStore = copy(rows = rows + (entity to list(entity).filterNot { it.id == id }))

    /** 集計・式が読む形(数の field は数に)。 */
    fun records(entity: String, model: jp.kz2601.metamon.dsl.records.Entity?): List<Record> = list(entity).mapIndexed { i, r ->
        Record(r.id, r.data.mapValues { (k, v) -> typedValue(v, model?.field(k)?.type) }, createdAt = r.createdAt ?: "%06d".format(i), author = r.author ?: "me")
    }

    companion object {
        /** 記録の値 → モックの文字列(配列・オブジェクトは JSON のまま)。 */
        fun cell(v: jp.kz2601.metamon.dsl.records.JSONValue): String = when (v) {
            is jp.kz2601.metamon.dsl.records.JSONValue.Arr, is jp.kz2601.metamon.dsl.records.JSONValue.Obj -> v.toJsonElement().toString()
            else -> v.displayString
        }

        /** `{"<entity>": [{"id", "data"}]}`(適合テストの case.json の records と同じ形)を読む。開発・スクリーンショット用。 */
        fun fromJson(text: String): MockStore {
            val root = Json.parseToJsonElement(text).jsonObject
            val obj = (root["records"] as? JsonObject) ?: root
            val rows = obj.mapValues { (_, list) ->
                list.jsonArray.map { e ->
                    val r = Record.from(e)
                    Row(r.id, r.data.mapValues { cell(it.value) }, r.createdAt ?: "", r.author)
                }
            }
            return MockStore(rows)
        }
    }
}
