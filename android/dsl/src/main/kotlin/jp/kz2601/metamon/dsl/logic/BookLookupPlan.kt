package jp.kz2601.metamon.dsl.logic

import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.FieldCoding
import jp.kz2601.metamon.dsl.records.FieldType
import jp.kz2601.metamon.dsl.records.JSONValue

/** `lookupBook.bookLookup` の中身(iOS の `BookLookupSpec`・Go の `dsl.BookLookup`)。フォームの entity の field id。 */
data class BookLookupFields(
    val isbnField: String,
    val titleField: String? = null,
    val authorsField: String? = null,
    val publisherField: String? = null,
    val publishedDateField: String? = null,
)

/** `POST /v1/lookup/book` の応答(iOS の `BookInfo`)。 */
data class BookInfo(
    val isbn: String,
    val title: String,
    val authors: String? = null,
    val publisher: String? = null,
    /** YYYY-MM-DD / YYYY-MM / YYYY */
    val publishedDate: String? = null,
    val coverUrl: String? = null,
    val source: Source,
) {
    data class Source(val name: String, val url: String, val license: String? = null)
}

/** ISBN の確かめ(サーバーの `books.NormalizeISBN` と同じ規則)と、見つかった本 → フォームの値。iOS の Records/BookLookupPlan.swift。 */
object BookLookupPlan {
    /** ISBN-10 / ISBN-13(ハイフン・空白は可)を ISBN-13 の数字に。検査数字が合わない・978 / 979 で始まらない時は null。 */
    fun normalizeISBN(raw: String): String? {
        if (raw.graphemeCount() > 40) return null
        val s = StringBuilder()
        for (ch in raw) {
            when (ch) {
                in '0'..'9' -> s.append(ch)
                'x', 'X' -> s.append('X')
                '-', ' ', '　' -> Unit
                else -> return null
            }
        }
        val d = s.toString()
        return when (d.length) {
            10 -> {
                var sum = 0
                for ((i, c) in d.withIndex()) {
                    val v = if (c == 'X') {
                        if (i != 9) return null
                        10
                    } else {
                        c - '0'
                    }
                    sum += v * (10 - i)
                }
                if (sum % 11 != 0) null else isbn13("978" + d.substring(0, 9))
            }
            13 -> {
                if ('X' in d || !(d.startsWith("978") || d.startsWith("979"))) null
                else if (isbn13(d.substring(0, 12)) == d) d else null
            }
            else -> null
        }
    }

    private fun isbn13(twelve: String): String {
        var sum = 0
        for ((i, c) in twelve.withIndex()) {
            val v = c - '0'
            sum += if (i % 2 == 1) v * 3 else v
        }
        return twelve + ((10 - sum % 10) % 10)
    }

    /** 埋める値(field id → 値)。entity に無い field・型の合わない field・空の値は入れない。出版日は date の field なら日付にする。 */
    fun values(spec: BookLookupFields, book: BookInfo, entity: Entity): Map<String, JSONValue> {
        val out = linkedMapOf<String, JSONValue>()
        fun put(id: String?, value: String?, allowDate: Boolean = false) {
            if (id == null) return
            val f = entity.field(id) ?: return
            if (f.computed != null) return
            val v = value?.trimWhitespace()?.takeIf { it.isNotEmpty() } ?: return
            when {
                f.type == FieldType.STRING || f.type == FieldType.TEXT -> out[id] = JSONValue.Str(v)
                f.type == FieldType.DATE && allowDate -> dateString(v)?.let { out[id] = JSONValue.Str(it) }
            }
        }
        put(spec.titleField, book.title)
        put(spec.authorsField, book.authors)
        put(spec.publisherField, book.publisher)
        put(spec.publishedDateField, book.publishedDate, allowDate = true)
        return out
    }

    /** YYYY / YYYY-MM / YYYY-MM-DD → YYYY-MM-DD(足りない所は 01)。 */
    internal fun dateString(s: String): String? {
        val parts = s.split('-').filter { it.isNotEmpty() }
        if (parts.size !in 1..3 || parts[0].length != 4 || !parts.all { p -> p.all { it in '0'..'9' } }) return null
        val out = (parts + List(3 - parts.size) { "01" }).joinToString("-")
        return if (FieldCoding.strictDay(out) == null) null else out
    }
}
