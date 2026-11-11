package jp.kz2601.metamon.renderer.form

import jp.kz2601.metamon.dsl.Component
import jp.kz2601.metamon.dsl.ComponentType
import jp.kz2601.metamon.dsl.Screen
import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.JSONValue
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.jsonObject
import kotlinx.serialization.json.jsonPrimitive
import java.io.File

/** 下書きの置き場(JSON の文字列 1 つ)。null を書いたら消す。 */
interface FormDraftStorage {
    fun read(): String?
    fun write(text: String?)

    /** 端末に残さない(テスト・プレビュー)。 */
    class InMemory : FormDraftStorage {
        var text: String? = null
        override fun read(): String? = text
        override fun write(text: String?) {
            this.text = text
        }
    }

    /** アプリの files の下の 1 ファイル(バックアップに乗せる置き場ではない)。 */
    class OnDisk(private val file: File) : FormDraftStorage {
        override fun read(): String? = runCatching { file.takeIf { it.isFile }?.readText() }.getOrNull()
        override fun write(text: String?) {
            runCatching {
                if (text == null) {
                    file.delete()
                } else {
                    file.parentFile?.mkdirs()
                    val tmp = File(file.path + ".tmp")
                    tmp.writeText(text)
                    if (!tmp.renameTo(file)) {
                        file.writeText(text)
                        tmp.delete()
                    }
                }
            }
        }
    }
}

/**
 * プロセスが落とされた後にフォームの書きかけを戻す(#1185)。iOS には無い(Android は背面で落とされやすいので足す)。
 *
 * - 置く: Activity の保存状態を書く時([persist])に、開いているフォームのうち未保存の入力があるものだけ。
 *   キーは appId + route の id(スタック / 画面 / recordId)。伏せ字(secureField)の欄と、写真・録音などの大きな値(data URL)は入れない。
 * - 戻す: 画面の位置を戻せた時だけ [arm] し、同じ route・同じ記録のフォームを作った時に [restore] で当てる。
 *   DSL の field(id と型)が変わっていたら捨てる。
 * - 消す: 保存・「破棄」([forget])、端末のデータを全部消す・アプリの削除([clear] / [forgetApp])、戻さずに起動した時([clear])。
 */
class FormDraftRestore(private val storage: FormDraftStorage) {
    private class Open(val form: FormDraft, val excluded: Set<String>)

    private val open = mutableMapOf<String, Open>()
    private var armed = false

    /** 画面に出ているフォームを控える([excluded] は置かない field)。 */
    @Synchronized
    fun attach(routeId: String, form: FormDraft, excluded: Set<String>) {
        open[routeId] = Open(form, excluded)
    }

    @Synchronized
    fun detach(routeId: String, form: FormDraft) {
        if (open[routeId]?.form === form) open.remove(routeId)
    }

    /** 今開いている未保存のフォームを置き場に書き直す(無ければ消す)。 */
    @Synchronized
    fun persist(appId: String) {
        val drafts = open.mapNotNull { (routeId, o) -> encodeDraft(o)?.let { routeId to it } }.toMap()
        var text = if (drafts.isEmpty()) null else document(appId, drafts)
        if (text != null && text.length > MAX_TOTAL_CHARS) text = null // 大きすぎるなら残さない
        storage.write(text)
    }

    /** 画面の位置を戻せた([appId] のアプリ)。置いてある下書きを当ててよい。 */
    @Synchronized
    fun arm(appId: String) {
        armed = read()?.appId == appId
        if (!armed) storage.write(null)
    }

    /**
     * 作ったばかりの [form] に、同じ route・同じ記録の下書きがあれば当てる。当てたら true。
     * 開いた時の値はそのまま(未保存の入力として扱う = 離れる時に破棄の確認が出る)。
     */
    @Synchronized
    fun restore(routeId: String, form: FormDraft): Boolean {
        if (!armed || form.isReadOnly) return false
        val saved = read()?.drafts?.get(routeId) ?: return false
        if (saved.entity != form.entity.id || saved.recordId != form.recordId) return false
        if (saved.signature != signature(form.entity)) {
            forgetKey(routeId) // DSL の field が変わった
            return false
        }
        val fields = form.entity.storedFields.map { it.id }.toSet()
        saved.values.filterKeys { it in fields }.forEach { (k, v) -> form.values[k] = v }
        return true
    }

    /** 保存した・「破棄」した [form] の下書きを消す。 */
    @Synchronized
    fun forget(form: FormDraft) {
        open.entries.filter { it.value.form === form }.forEach { forgetKey(it.key) }
    }

    /** 削除したアプリの下書きを消す。 */
    @Synchronized
    fun forgetApp(appId: String) {
        if (read()?.appId == appId) storage.write(null)
    }

    /** 全部消す(端末のデータを消す・戻さずに起動した)。 */
    @Synchronized
    fun clear() {
        armed = false
        storage.write(null)
    }

    private fun forgetKey(routeId: String) {
        val doc = read() ?: return
        val rest = doc.drafts - routeId
        if (rest.size == doc.drafts.size) return
        storage.write(if (rest.isEmpty()) null else document(doc.appId, rest.mapValues { encodeSaved(it.value) }))
    }

    // 置き場の形

    private class Saved(val entity: String, val recordId: String?, val signature: String, val values: Map<String, JSONValue>)
    private class Doc(val appId: String, val drafts: Map<String, Saved>)

    private fun encodeDraft(o: Open): JsonObject? {
        val form = o.form
        if (!form.hasUnsavedChanges) return null
        val values = form.entity.storedFields
            .filter { it.id !in o.excluded }
            .mapNotNull { f -> form.values[f.id]?.takeIf(::isStorable)?.let { f.id to it } }
            .toMap()
        return encodeSaved(Saved(form.entity.id, form.recordId, signature(form.entity), values))
    }

    private fun encodeSaved(s: Saved): JsonObject = JsonObject(
        mapOf(
            "entity" to JsonPrimitive(s.entity),
            "recordId" to (s.recordId?.let(::JsonPrimitive) ?: JsonNull),
            "signature" to JsonPrimitive(s.signature),
            "values" to JsonObject(s.values.mapValues { it.value.toJsonElement() }),
        ),
    )

    private fun document(appId: String, drafts: Map<String, JsonObject>): String = JsonObject(
        mapOf("version" to JsonPrimitive(1), "appId" to JsonPrimitive(appId), "drafts" to JsonObject(drafts)),
    ).toString()

    private fun read(): Doc? = runCatching {
        val o = Json.parseToJsonElement(storage.read() ?: return null).jsonObject
        val appId = o["appId"]?.jsonPrimitive?.contentOrNull ?: return null
        val drafts = o["drafts"]?.jsonObject.orEmpty().mapValues { (_, e) ->
            val d = e.jsonObject
            Saved(
                entity = d["entity"]!!.jsonPrimitive.content,
                recordId = d["recordId"]?.jsonPrimitive?.contentOrNull,
                signature = d["signature"]!!.jsonPrimitive.content,
                values = d["values"]?.jsonObject.orEmpty().mapValues { JSONValue.from(it.value) },
            )
        }
        Doc(appId, drafts)
    }.getOrNull()

    companion object {
        /** 1 つの文字の値の上限。超えるもの(写真・録音・手書きの data URL など)は置かない。 */
        const val MAX_VALUE_CHARS = 4_096
        const val MAX_TOTAL_CHARS = 256 * 1_024

        /** 置かない field: 伏せ字(secureField)が書く欄。 */
        fun excludedFields(screen: Screen, entity: Entity): Set<String> {
            fun walk(c: Component): Sequence<Component> = sequenceOf(c) + c.children.asSequence().flatMap(::walk)
            return screen.components.asSequence().flatMap(::walk)
                .filter { it.type == ComponentType.SECURE_FIELD && (it.entity == null || it.entity == entity.id) }
                .mapNotNull { it.field }
                .toSet()
        }

        /** DSL の field が変わったか見るための印(id と型)。 */
        fun signature(entity: Entity): String = entity.storedFields.joinToString(",") { "${it.id}:${it.type.wire}" }

        /** 小さな値だけ置く(data URL・長い文字は参照ではなく中身なので置かない)。 */
        fun isStorable(v: JSONValue): Boolean = when (v) {
            is JSONValue.Str -> v.value.length <= MAX_VALUE_CHARS && !v.value.startsWith("data:")
            is JSONValue.Arr -> v.items.all(::isStorable)
            is JSONValue.Obj -> v.fields.values.all(::isStorable)
            else -> true
        }
    }
}
