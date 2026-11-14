package jp.kz2601.metamon.dsl.logic

import jp.kz2601.metamon.dsl.AppDSL
import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.Field
import jp.kz2601.metamon.dsl.records.FieldCoding
import jp.kz2601.metamon.dsl.records.FieldType
import jp.kz2601.metamon.dsl.records.JSONValue
import jp.kz2601.metamon.dsl.records.Record
import jp.kz2601.metamon.dsl.records.entityModels
import java.util.Locale
import java.util.regex.Pattern

/**
 * ショートカットの「記録を追加」(#80)用: 一言(「ランチ 800円」)から entity の 1 レコードを組み立てる。iOS の Records/QuickRecord.swift。
 * DSL を変えずに、どの生成アプリでも動くよう Field の型だけで当てはめる:
 * - タイトル系(titleField → 最初の string/text): 数値を除いた文言。enum ならオプションに一致する値
 * - 数値 1 つ(単位の合う数値 → 最初の required な数値 → 最初の数値): 文中の数値(またはパラメータ)
 * - ref: 参照先レコードの見出し(`refChoices`)に一致すればその id(1 文字の見出しは語単位でだけ)
 * - それ以外: フォームと同じ既定値(`FieldCoding.defaultValue`)
 */
object QuickRecord {
    data class Parsed(
        val title: String?,
        val number: Double?,
        /** 数値の直後の単位(円/kg/回…)。数値項目の選択に使う */
        val unit: String? = null,
    )

    data class Draft(
        /** appconfig に送る `data`(正規化済み) */
        val data: Map<String, JSONValue>,
        /** 埋められなかった required 項目の表示名 */
        val missing: List<String>,
        /** 応答用の短い要約(例: 「ランチ ¥800」) */
        val summary: String,
        /** 文中の語で当てた ref(「カテゴリ: 食費」)。既定値のままの ref は含めない */
        val refs: List<String> = emptyList(),
    )

    /** ref の候補 1 件(参照先レコードの id と見出し)。 */
    data class RefChoice(val id: String, val title: String)

    // 末尾側の数値を優先(「3月の食費 1200円」→ 1200)。\d \s は Unicode(NSRegularExpression と同じ)
    private val numberPattern: Pattern =
        Pattern.compile("""[¥$]?\s*(\d[\d,]*(?:\.\d+)?)([^\s\d、。,.]{0,3})""", Pattern.UNICODE_CHARACTER_CLASS)

    private val particles = setOf('を', 'に', 'で', 'の', 'は', 'が', 'と')

    /** 「ランチ 800円」→ (ランチ, 800, 円)。全角数字・桁区切り・小数・¥/円/kg/回 などの単位を許す。数値が無ければ title だけ。 */
    fun parse(raw: String): Parsed {
        val text = halfwidthDigits(raw).trimWhitespace()
        if (text.isEmpty()) return Parsed(null, null)
        val m = numberPattern.matcher(text)
        var start = -1
        var end = -1
        var numStart = -1
        var numEnd = -1
        var unitStart = -1
        var unitEnd = -1
        while (m.find()) {
            start = m.start(); end = m.end()
            numStart = m.start(1); numEnd = m.end(1)
            unitStart = m.start(2); unitEnd = m.end(2)
        }
        if (start < 0) return Parsed(cleanTitle(text), null)
        val number = text.substring(numStart, numEnd).replace(",", "").toDoubleOrNull() ?: return Parsed(cleanTitle(text), null)
        // 単位っぽい後続(円/kg/回…)は落とすが、「800円のランチ」の「のラ」のような助詞+語は残す
        var restFrom = end
        var unitText: String? = null
        val whole = text.substring(start, end)
        if (whole.startsWith("¥") || whole.startsWith("$")) unitText = "円"
        if (unitStart >= 0) {
            val unit = text.substring(unitStart, unitEnd)
            val kept = unit.takeWhile { it !in particles }
            restFrom = unitStart + kept.length
            if (kept.isNotEmpty()) unitText = kept
        }
        val rest = text.substring(0, start) + " " + text.substring(restFrom)
        return Parsed(cleanTitle(rest), number, unitText)
    }

    /** 全角の数字・記号だけ半角に(カタカナは変えない)。 */
    internal fun halfwidthDigits(s: String): String {
        val sb = StringBuilder(s.length)
        for (c in s) {
            sb.append(
                when (c.code) {
                    in 0xFF10..0xFF19, 0xFF0C, 0xFF0E -> (c.code - 0xFEE0).toChar()
                    0xFFE5 -> '¥'
                    0x3000 -> ' '
                    else -> c
                },
            )
        }
        return sb.toString()
    }

    private fun cleanTitle(s: String): String? {
        var t = s.trim { isSwiftWhitespace(it) || it in "、。,." }
        while (t.isNotEmpty() && t.first() in particles) t = t.drop(1).trimSpaces()
        while (t.isNotEmpty() && t.last() in particles) t = t.dropLast(1).trimSpaces()
        t = t.split(Regex("""[\s   -     　\u0085]+""")).filter { it.isNotEmpty() }.joinToString(" ")
        return t.ifEmpty { null }
    }

    /** タイトルに当てる項目: titleField(string/text/enum)→ 最初の string/text。 */
    fun titleField(entity: Entity): Field? {
        entity.titleField?.let { id ->
            entity.field(id)?.takeIf { it.type == FieldType.STRING || it.type == FieldType.TEXT || it.type == FieldType.ENUM }?.let { return it }
        }
        return entity.fields.firstOrNull { it.type == FieldType.STRING || it.type == FieldType.TEXT }
    }

    /** 数値に当てる項目: 単位が一致する数値(「20回」→ unit 回、「800円」→ currency)→ 最初の required な数値 → 最初の数値。 */
    fun numberField(entity: Entity, unit: String? = null): Field? {
        if (!unit.isNullOrEmpty()) {
            val isYen = unit in setOf("円", "¥", "$")
            entity.fields.firstOrNull { f ->
                f.type.isNumeric && ((f.unit?.let { unit.startsWith(it) || it.startsWith(unit) } ?: false) || (isYen && f.type == FieldType.CURRENCY))
            }?.let { return it }
        }
        return entity.fields.firstOrNull { it.type.isNumeric && it.required == true } ?: entity.fields.firstOrNull { it.type.isNumeric }
    }

    /** 「記録を追加」の対象に出す entity(iOS の `recordableEntities(in:)`)。フォーム画面(`screen.entity`)を持つものを先に、項目の無いものは除く。 */
    fun recordableEntities(dsl: AppDSL): List<Entity> = recordableEntities(dsl.entityModels(), dsl.formEntityIds)

    /** [recordableEntities] の本体。`formEntityIds` はフォーム画面の entity id。 */
    internal fun recordableEntities(entities: List<Entity>, formEntityIds: Set<String>): List<Entity> {
        val usable = entities.filter { it.fields.isNotEmpty() }
        return usable.filter { it.id in formEntityIds } + usable.filter { it.id !in formEntityIds }
    }

    /** entity の ref 項目ごとの候補(field id → 参照先レコードの見出し)。`records` は参照先 entity id → そのレコード。 */
    fun refChoices(entity: Entity, entities: List<Entity>, records: Map<String, List<Record>>, locale: Locale = Locale.getDefault()): Map<String, List<RefChoice>> {
        val out = linkedMapOf<String, List<RefChoice>>()
        for (f in entity.fields) {
            if (f.type != FieldType.REF) continue
            val target = f.refEntity?.let { id -> entities.firstOrNull { it.id == id } } ?: continue
            val tf = target.displayTitleField ?: continue
            out[f.id] = (records[target.id] ?: emptyList()).mapNotNull { r ->
                val title = FieldCoding.display(r[tf], target.field(tf), locale)
                if (title.isEmpty()) null else RefChoice(r.id, title)
            }
        }
        return out
    }

    /** `text`(一言)と `number`(明示パラメータ。null なら文中の数値)から 1 レコード分の `data` を作る。 */
    fun build(
        entity: Entity,
        text: String?,
        number: Double? = null,
        refChoices: Map<String, List<RefChoice>> = emptyMap(),
        locale: Locale = Locale.getDefault(),
    ): Draft {
        val parsed = parse(text ?: "")
        val num = number ?: parsed.number
        val values = linkedMapOf<String, JSONValue>()
        for (f in entity.fields) values[f.id] = FieldCoding.defaultValue(f)

        val titleF = titleField(entity)
        val numberF = numberField(entity, parsed.unit)
        val rawText = text ?: ""
        // 文中にオプション名がある enum は全部それに合わせる(「交通費 300円」→ category=交通費)
        for (f in entity.fields) {
            if (f.type != FieldType.ENUM) continue
            matchOption(rawText, f.options ?: emptyList())?.let { values[f.id] = JSONValue.Str(it) }
        }
        // ref も同じ最長一致で参照先の見出しに合わせ、id を入れる
        val refs = mutableListOf<String>()
        for (f in entity.fields) {
            if (f.type != FieldType.REF) continue
            val choice = matchRef(rawText, refChoices[f.id] ?: emptyList()) ?: continue
            values[f.id] = JSONValue.Str(choice.id)
            refs.add("${f.name}: ${choice.title}")
        }
        val title = parsed.title
        if (titleF != null && title != null) {
            if (titleF.type == FieldType.ENUM) {
                val hit = matchOption(title, titleF.options ?: emptyList())
                if (hit != null) values[titleF.id] = JSONValue.Str(hit)
                else if (titleF.options.isNullOrEmpty()) values[titleF.id] = JSONValue.Str(title)
            } else {
                values[titleF.id] = JSONValue.Str(title)
            }
        }
        if (numberF != null && num != null) values[numberF.id] = JSONValue.Num(num)

        val data = linkedMapOf<String, JSONValue>()
        for (f in entity.fields) {
            val v = FieldCoding.normalize(values[f.id] ?: JSONValue.Null, f)
            if (v != null && !v.isNull) data[f.id] = v
        }
        val missing = entity.fields.filter { it.required == true && data[it.id] == null }.map { it.name }
        val parts = listOfNotNull(titleF, numberF).mapNotNull { f -> data[f.id]?.let { FieldCoding.display(it, f, locale) } }
        return Draft(data, missing, parts.filter { it.isNotEmpty() }.joinToString(" "), refs)
    }

    /** これより短い ref の見出しは、空白で区切った語と一致する時だけ当てる(「電車」に「車」を当てない)。 */
    internal const val MIN_REF_TITLE_LENGTH = 2

    internal fun matchRef(text: String, choices: List<RefChoice>): RefChoice? {
        val words = text.lowercase(Locale.ROOT).split(Regex("""\s+""")).filter { it.isNotEmpty() }.toSet()
        val usable = choices.filter { it.title.graphemeCount() >= MIN_REF_TITLE_LENGTH || it.title.lowercase(Locale.ROOT) in words }
        val hit = matchOption(text, usable.map { it.title }) ?: return null
        return usable.firstOrNull { it.title == hit }
    }

    /** 最長一致のオプション(「ベンチプレス」と「ベンチ」なら長い方・同じ長さなら先)。 */
    internal fun matchOption(text: String, options: List<String>): String? {
        val t = text.lowercase(Locale.ROOT)
        return options.filter { it.isNotEmpty() && t.contains(it.lowercase(Locale.ROOT)) }.maxByOrNull { it.graphemeCount() }
    }
}
