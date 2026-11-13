package jp.kz2601.metamon.desktop.io

import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.Field
import jp.kz2601.metamon.dsl.records.FieldType
import jp.kz2601.metamon.dsl.records.JSONValue
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.intOrNull
import java.awt.datatransfer.DataFlavor
import java.awt.datatransfer.Transferable
import java.awt.datatransfer.UnsupportedFlavorException

/**
 * 記録 1 件を項目つきでコピーする形(#2986)。iOS の `RecordClipboard` と同じ形で、
 * 「項目名: 値」の文字(他のアプリ向け)と貼り付け用の JSON(Tailor It 向け)の 2 つで持つ。純 Kotlin(AWT は [Clip] だけ)。
 *
 * 載せる項目は保存フィールドのうち、hidden・暗号化・端末のファイル(photo / video / audio)・
 * アプリが違うと意味を持たない参照(ref / refs / member / members)以外。算出項目は保存フィールドでないので載らない。
 * null・型の合わない値も載せない。
 */
object RecordClipboard {
    const val VERSION = 1

    /** 読む・書くデータの上限(バイト)。超えたら読まない。 */
    const val MAX_BYTES = 256 * 1024

    /** 載せる項目の上限。 */
    const val MAX_FIELDS = 200

    private const val MAX_ID_LENGTH = 128
    private const val MAX_LABEL_LENGTH = 100

    data class FieldValue(val id: String, val name: String, val type: String, val value: JSONValue)

    data class Payload(
        val appId: String,
        val appName: String,
        val entityId: String,
        val entityName: String,
        val fields: List<FieldValue>,
        val v: Int = VERSION,
    )

    private val excludedTypes = setOf(
        FieldType.PHOTO, FieldType.VIDEO, FieldType.AUDIO,
        FieldType.REF, FieldType.REFS, FieldType.MEMBER, FieldType.MEMBERS,
    )

    /**
     * コピーの対象になる field。DSL の `encrypted` は :dsl の Field が持たないので、暗号化した項目の id は [encryptedFieldIds] で渡す。
     */
    fun copyableFields(entity: Entity, encryptedFieldIds: Set<String> = emptySet()): List<Field> =
        entity.storedFields.filter { !it.hidden && it.id !in encryptedFieldIds && it.type !in excludedTypes }

    /** 1 件の記録から載せるものを作る。載せる項目が 1 つも無ければ null(コピーの項目を出さない)。 */
    fun make(
        appId: String,
        appName: String,
        entity: Entity,
        data: Map<String, JSONValue>,
        encryptedFieldIds: Set<String> = emptySet(),
    ): Payload? {
        val fields = copyableFields(entity, encryptedFieldIds).take(MAX_FIELDS).mapNotNull { f ->
            val v = data[f.id] ?: return@mapNotNull null
            if (v.isNull || !accepts(v, f.type)) return@mapNotNull null
            FieldValue(f.id, f.name, f.type.wire, v)
        }
        if (fields.isEmpty()) return null
        return Payload(appId, appName, entity.id, entity.name, fields)
    }

    /** 値が field の型に合うか(null・member などは受けない)。 */
    fun accepts(value: JSONValue, type: FieldType): Boolean = when (type) {
        FieldType.INTEGER, FieldType.NUMBER, FieldType.CURRENCY -> value is JSONValue.Num
        FieldType.BOOLEAN -> value is JSONValue.Bool
        FieldType.STRING, FieldType.TEXT, FieldType.DATE, FieldType.DATETIME, FieldType.ENUM, FieldType.PHOTO,
        FieldType.VIDEO, FieldType.AUDIO, FieldType.LOCATION, FieldType.REF,
        -> value is JSONValue.Str
        FieldType.REFS -> value is JSONValue.Arr
        FieldType.MEMBER, FieldType.MEMBERS -> false
    }

    // 文字

    /**
     * 文字として貼る先(メモ・メール)向けの「名前: 値」の行。[display] は値を画面と同じ書き方にする(無ければ素の値)。
     * 値が空の項目は出さず、名前が空なら値だけ。
     */
    fun plainText(payload: Payload, display: (FieldValue) -> String = { rawText(it.value) }): String =
        payload.fields.mapNotNull { f ->
            val value = display(f).trim()
            if (value.isEmpty()) null else if (f.name.isEmpty()) value else "${f.name}: $value"
        }.joinToString("\n")

    /** 値の素の文字。整数は小数点なし(1e15 未満)、配列は ", " でつなぐ。日付はサーバーの文字のまま。 */
    fun rawText(v: JSONValue): String = when (v) {
        is JSONValue.Str -> v.value
        is JSONValue.Num -> JSONValue.numberText(v.value)
        is JSONValue.Bool -> if (v.value) "true" else "false"
        is JSONValue.Arr -> v.items.joinToString(", ") { rawText(it) }
        JSONValue.Null, is JSONValue.Obj -> ""
    }

    // JSON

    /** 貼り付け用の JSON。上限([MAX_BYTES])を超えたら null。 */
    fun encode(payload: Payload): String? {
        val obj = JsonObject(
            mapOf(
                "v" to JsonPrimitive(payload.v),
                "appId" to JsonPrimitive(payload.appId),
                "appName" to JsonPrimitive(payload.appName),
                "entityId" to JsonPrimitive(payload.entityId),
                "entityName" to JsonPrimitive(payload.entityName),
                "fields" to JsonArray(
                    payload.fields.map {
                        JsonObject(
                            mapOf(
                                "id" to JsonPrimitive(it.id),
                                "name" to JsonPrimitive(it.name),
                                "type" to JsonPrimitive(it.type),
                                "value" to element(it.value),
                            ),
                        )
                    },
                ),
            ),
        )
        val text = Json.encodeToString(JsonElement.serializer(), obj)
        return text.takeIf { it.toByteArray(Charsets.UTF_8).size <= MAX_BYTES }
    }

    /**
     * クリップボードの JSON を読む。大きすぎる・新しすぎる・形や id が壊れたものは null。名前は上限に切り詰め、壊れた項目は捨てる。
     */
    fun decode(text: String): Payload? {
        if (text.toByteArray(Charsets.UTF_8).size > MAX_BYTES) return null
        val o = runCatching { Json.parseToJsonElement(text) }.getOrNull() as? JsonObject ?: return null
        val v = (o["v"] as? JsonPrimitive)?.intOrNull ?: return null
        if (v > VERSION) return null
        val appId = o.string("appId") ?: return null
        val entityId = o.string("entityId") ?: return null
        if (!validId(entityId) || appId.length > MAX_ID_LENGTH) return null
        val raw = o["fields"] as? JsonArray ?: return null
        val fields = raw.take(MAX_FIELDS).mapNotNull { e ->
            val f = e as? JsonObject ?: return@mapNotNull null
            val id = f.string("id")?.takeIf(::validId) ?: return@mapNotNull null
            val type = f.string("type")?.takeIf(::validId) ?: return@mapNotNull null
            val value = f["value"]?.let(JSONValue::from) ?: return@mapNotNull null
            FieldValue(id, (f.string("name") ?: "").clip(), type, value)
        }
        return Payload(
            appId = appId,
            appName = (o.string("appName") ?: "").clip(),
            entityId = entityId,
            entityName = (o.string("entityName") ?: "").clip(),
            fields = fields,
            v = v,
        )
    }

    private fun element(v: JSONValue): JsonElement = when (v) {
        // 整数は 32.0 でなく 32 と書く
        is JSONValue.Num -> if (v.value == Math.rint(v.value) && Math.abs(v.value) < 1e15) JsonPrimitive(v.value.toLong()) else JsonPrimitive(v.value)
        is JSONValue.Arr -> JsonArray(v.items.map(::element))
        is JSONValue.Obj -> JsonObject(v.fields.mapValues { element(it.value) })
        JSONValue.Null -> JsonNull
        else -> v.toJsonElement()
    }

    private fun JsonObject.string(key: String): String? = (this[key] as? JsonPrimitive)?.takeIf { it.isString }?.contentOrNull

    private fun validId(s: String) = s.isNotEmpty() && s.length <= MAX_ID_LENGTH

    private fun String.clip(): String = trim().take(MAX_LABEL_LENGTH)

    // クリップボード

    /** Tailor It 向けの型。同じ型を読む側がこの名前で探す。 */
    val jsonFlavor: DataFlavor = DataFlavor("application/vnd.tailorit.record+json;class=java.lang.String", "Tailor It record")

    /** 文字([DataFlavor.stringFlavor])と JSON([jsonFlavor])を両方持つ。文字が先(他のアプリは文字を読む)。 */
    class Clip(val text: String, val json: String) : Transferable {
        override fun getTransferDataFlavors(): Array<DataFlavor> = arrayOf(DataFlavor.stringFlavor, jsonFlavor)

        override fun isDataFlavorSupported(flavor: DataFlavor): Boolean = flavor == DataFlavor.stringFlavor || flavor == jsonFlavor

        override fun getTransferData(flavor: DataFlavor): Any = when (flavor) {
            DataFlavor.stringFlavor -> text
            jsonFlavor -> json
            else -> throw UnsupportedFlavorException(flavor)
        }
    }

    /** クリップボードに入れる形。載せるものが無い・大きすぎる時は null。 */
    fun clip(payload: Payload, display: (FieldValue) -> String = { rawText(it.value) }): Clip? {
        val json = encode(payload) ?: return null
        return Clip(plainText(payload, display), json)
    }

    /** クリップボードから読み戻す(Tailor It の JSON だけ。文字だけの中身は null)。 */
    fun read(t: Transferable): Payload? {
        if (!t.isDataFlavorSupported(jsonFlavor)) return null
        val text = runCatching { t.getTransferData(jsonFlavor) as? String }.getOrNull() ?: return null
        return decode(text)
    }
}
