package jp.kz2601.metamon.desktop.io

import jp.kz2601.metamon.dsl.logic.RecordImport
import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.Field
import jp.kz2601.metamon.dsl.records.FieldCoding
import jp.kz2601.metamon.dsl.records.FieldType
import jp.kz2601.metamon.dsl.records.JSONValue
import kotlin.math.abs

/**
 * 項目つきでコピーした記録([RecordClipboard.Payload])を、別のフォームに貼る時の項目の合わせ方(#2987)。
 * iOS の `RecordClipboard.paste` / `coerce` と同じ規則。純 Kotlin(画面・クリップボードに触らない)。
 *
 * - 同じアプリの同じ entity なら field の id と型で合わせ、それ以外(または合わない時)は名前(前後の空白・大文字小文字を無視)で合わせる
 * - 貼る先の 1 つの field には 1 回だけ入れる。型が変えられない値・選択肢に無い値は捨てる
 * - [isAdmin] が false なら管理者だけの項目には入れない
 */
object RecordPaste {
    /** [values] は貼る先の field id → 値。[matched] は入った項目(貼る先の名前・フォームの順)、[unmatched] は合わなかった項目(コピー元の名前・コピー元の順)。 */
    data class Result(val values: Map<String, JSONValue>, val matched: List<String>, val unmatched: List<String>)

    private val excludedTypes = setOf(
        FieldType.PHOTO, FieldType.VIDEO, FieldType.AUDIO,
        FieldType.REF, FieldType.REFS, FieldType.MEMBER, FieldType.MEMBERS,
    )

    /** integer / currency の上限(サーバーの RecordData と同じ)。 */
    private const val MAX_INTEGER = (Long.MAX_VALUE / 2).toDouble()

    fun paste(payload: RecordClipboard.Payload, entity: Entity, appId: String, isAdmin: Boolean = false): Result {
        val targets = RecordClipboard.copyableFields(entity).filter { isAdmin || !it.isAdminOnly }
        val sameEntity = payload.appId == appId && payload.entityId == entity.id
        val used = HashSet<String>()
        val values = LinkedHashMap<String, JSONValue>()
        val unmatched = ArrayList<String>()
        for (src in payload.fields) {
            val byId = if (sameEntity) targets.firstOrNull { it.id == src.id && it.id !in used } else null
            val candidates = listOfNotNull(byId) + targets.filter { it.id !in used && nameKey(it.name) == nameKey(src.name) && it.id != byId?.id }
            val placed = candidates.firstNotNullOfOrNull { t -> coerce(src.value, src.type, t)?.let { t to it } }
            if (placed == null) {
                unmatched += src.name.ifEmpty { src.id }
            } else {
                values[placed.first.id] = placed.second
                used += placed.first.id
            }
        }
        val matched = targets.filter { it.id in values }.map { it.name }
        return Result(values, matched, unmatched)
    }

    private fun nameKey(s: String) = s.trim().lowercase()

    /**
     * コピー元の値を貼る先の field の型に変える。変えられなければ null。
     * 文字(string ↔ text・enum → 文字。文字数は [RecordImport.characterLimit] まで)、
     * 数(integer → number / currency、整数の number → integer、currency は四捨五入、数字だけの文字 → 数)、選択肢(選べる値だけ)、
     * 日付・日時(読めるものを書き直す・日時 → 日付)、位置(緯度 90・経度 180 まで)、同じ型どうし。
     */
    fun coerce(value: JSONValue, sourceType: String, target: Field): JSONValue? {
        val src = FieldType.entries.firstOrNull { it.wire == sourceType } ?: return null
        if (src in excludedTypes || !RecordClipboard.accepts(value, src)) return null
        val t = target.type
        return when {
            value is JSONValue.Str && (src == FieldType.STRING || src == FieldType.TEXT || src == FieldType.ENUM) -> when (t) {
                FieldType.STRING, FieldType.TEXT ->
                    if (value.value.codePointCount(0, value.value.length) > RecordImport.characterLimit(target)) null else value
                FieldType.ENUM -> if (value.value in (target.options ?: emptyList())) value else null
                FieldType.INTEGER, FieldType.NUMBER, FieldType.CURRENCY ->
                    value.value.trim().toDoubleOrNull()?.takeIf { it.isFinite() }?.let { numeric(it, t) }
                else -> null
            }
            value is JSONValue.Num && src.isNumeric -> when (t) {
                FieldType.INTEGER, FieldType.NUMBER, FieldType.CURRENCY -> numeric(value.value, t)
                FieldType.STRING, FieldType.TEXT -> JSONValue.Str(JSONValue.numberText(value.value))
                else -> null
            }
            value is JSONValue.Bool && src == FieldType.BOOLEAN -> if (t == FieldType.BOOLEAN) value else null
            value is JSONValue.Str && src == FieldType.DATE ->
                if (t == FieldType.DATE) FieldCoding.date(value)?.let { JSONValue.Str(FieldCoding.encode(it, target)) } else null
            value is JSONValue.Str && src == FieldType.DATETIME ->
                if (t == FieldType.DATETIME || t == FieldType.DATE) FieldCoding.date(value)?.let { JSONValue.Str(FieldCoding.encode(it, target)) } else null
            value is JSONValue.Str && src == FieldType.LOCATION -> if (t == FieldType.LOCATION) location(value.value)?.let { JSONValue.Str(it) } else null
            else -> null
        }
    }

    /** integer は小数を断り、currency は四捨五入する。どちらも上限を超えたら断る。 */
    private fun numeric(n: Double, t: FieldType): JSONValue? {
        if (!n.isFinite()) return null
        return when (t) {
            FieldType.INTEGER -> if (n == Math.rint(n) && abs(n) <= MAX_INTEGER) JSONValue.Num(n) else null
            FieldType.CURRENCY -> Math.rint(n).takeIf { abs(it) <= MAX_INTEGER }?.let { JSONValue.Num(it) }
            else -> JSONValue.Num(n)
        }
    }

    /** "lat,lng"(緯度 90・経度 180 まで)。空白を外して返す。読めなければ null。 */
    private fun location(s: String): String? {
        val parts = s.split(',').map { it.trim().toDoubleOrNull() }
        if (parts.size != 2) return null
        val lat = parts[0] ?: return null
        val lng = parts[1] ?: return null
        if (!lat.isFinite() || !lng.isFinite() || abs(lat) > 90 || abs(lng) > 180) return null
        return s.replace(" ", "")
    }
}
