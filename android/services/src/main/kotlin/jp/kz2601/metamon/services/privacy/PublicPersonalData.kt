package jp.kz2601.metamon.services.privacy

import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive

/** 生成で新しく「誰でも見られる」記録に入った個人の情報の欄 1 つ(#253・iOS の `PublicPersonalField`)。 */
data class PublicPersonalField(
    /** entity id */
    val entity: String,
    val entityName: String,
    /** field id */
    val field: String,
    /** 表示名(ダイアログにそのまま出す) */
    val fieldName: String,
    /** name | phone | address | email | birthday | location(未知の値もそのまま) */
    val kind: String,
) {
    companion object {
        /** entity・field が無ければ null。名前が無ければ id で埋める。 */
        fun from(o: JsonObject): PublicPersonalField? {
            fun str(k: String) = (o[k] as? JsonPrimitive)?.takeIf { it.isString }?.content
            val entity = str("entity") ?: return null
            val field = str("field") ?: return null
            return PublicPersonalField(entity, str("entityName") ?: entity, field, str("fieldName") ?: field, str("kind") ?: "")
        }
    }
}

/**
 * 生成直後の「個人の情報も公開されます」の確認(#253・#899・iOS の `PublicDataPrompt`)の中身と、ボタンで generate に送る修正の指示文。
 * @param version 出した時のアプリの版(別の版に変わったら指示を送らない)
 */
class PublicDataPrompt private constructor(val version: Int, val fields: List<PublicPersonalField>) {
    /** ダイアログに出す欄の名前(重複を除き「・」でつなぐ。英語は ", ")。 */
    fun fieldNames(english: Boolean = false): String = fields.map { it.fieldName }.distinct().joinToString(if (english) ", " else "・")

    /** 「この欄を消す」の指示。例: 予約の「お名前」と「電話番号」の欄を消して */
    fun removeInstruction(english: Boolean = false): String {
        val parts = groups.map { (entityName, names) ->
            if (english) {
                "the " + names.joinToString(" and ") { "\"$it\"" } + (if (names.size > 1) " fields" else " field") + " from $entityName"
            } else {
                entityName + "の" + names.joinToString("と") { "「$it」" } + "の欄"
            }
        }
        return if (english) "Remove " + parts.joinToString(", and ") else parts.joinToString("と、") + "を消して"
    }

    /** 「自分だけにする」の指示(backend の共有の語の表で scope が remove になる)。例: 予約を自分だけの記録にして */
    fun privateInstruction(english: Boolean = false): String {
        val names = groups.map { it.first }
        return if (english) "Keep " + names.joinToString(" and ") + " to only me (make it private)" else names.joinToString("と") + "を自分だけの記録にして"
    }

    /** entity ごとの (entityName, 欄の名前)(出てきた順・重複なし)。 */
    private val groups: List<Pair<String, List<String>>>
        get() {
            val byEntity = linkedMapOf<String, Pair<String, MutableList<String>>>()
            for (f in fields) {
                val g = byEntity.getOrPut(f.entity) { f.entityName to mutableListOf() }
                if (f.fieldName !in g.second) g.second += f.fieldName
            }
            return byEntity.values.toList()
        }

    companion object {
        /** 欄が無ければ null(旧サーバーはフィールドを送らない = 出さない)。 */
        fun of(fields: List<PublicPersonalField>, version: Int): PublicDataPrompt? = if (fields.isEmpty()) null else PublicDataPrompt(version, fields)

        /** `MorphResult.publicPersonalData`(JSON のまま)から。壊れた欄は捨てる。 */
        fun fromMeta(raw: List<JsonObject>, version: Int): PublicDataPrompt? = of(raw.mapNotNull(PublicPersonalField::from), version)
    }
}
