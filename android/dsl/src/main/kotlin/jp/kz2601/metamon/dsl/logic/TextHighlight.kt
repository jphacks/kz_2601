package jp.kz2601.metamon.dsl.logic

import jp.kz2601.metamon.dsl.records.Entity
import jp.kz2601.metamon.dsl.records.FieldType
import java.text.Normalizer
import java.util.Locale

/**
 * `text.highlight` の語の決め方と照合。iOS の Model/TextHighlight.swift(`TextHighlight` の計算と `TextHighlightMatcher`)。
 * 部品の値そのもの(words / field / style)は生成した型が持つので、ここは計算だけ。
 * 照合は大文字小文字・全角半角を区別せず(NFKC)、日本語は語の一部でも合う。
 */
object HighlightWords {
    const val MAX_WORDS = 20
    const val MAX_WORD_LENGTH = 50

    private val separators = Regex("[,、\n\r\u000B\u000C\u0085  ]")

    /**
     * 印をつける語(空・重複を除き、先頭から [MAX_WORDS] 個)。
     * `words` があればそれ、無ければ `field` の今の値 `fieldValue` を「,」「、」改行で区切る。
     */
    fun resolve(words: List<String>?, field: String?, fieldValue: String?): List<String> {
        val raw = when {
            !words.isNullOrEmpty() -> words
            field != null && fieldValue != null -> Normalizer.normalize(fieldValue, Normalizer.Form.NFKC).split(separators).filter { it.isNotEmpty() }
            else -> emptyList()
        }
        val seen = mutableSetOf<String>()
        val out = mutableListOf<String>()
        for (w in raw) {
            val t = w.trimSpaces()
            if (t.isEmpty() || t.graphemeCount() > MAX_WORD_LENGTH || !seen.add(fold(t))) continue
            out.add(t)
            if (out.size == MAX_WORDS) break
        }
        return out
    }

    /**
     * text の `{{id}}` をレコードの値に置き換えるか: 知っている・hidden でない・ふつうの値の field だけ。
     * Go `dsl.TextSubstitutes` と同じ。
     */
    fun substitutes(entity: Entity?, id: String): Boolean {
        val f = entity?.field(id) ?: return false
        if (f.hidden) return false
        return when (f.type) {
            FieldType.STRING, FieldType.TEXT, FieldType.INTEGER, FieldType.NUMBER, FieldType.BOOLEAN,
            FieldType.DATE, FieldType.DATETIME, FieldType.ENUM, FieldType.CURRENCY -> true
            else -> false
        }
    }

    /** 照合用の形: NFKC + 小文字。 */
    fun fold(s: String): String = Normalizer.normalize(s, Normalizer.Form.NFKC).lowercase(Locale.ROOT)

    /** `text` の中で `words` に合う範囲(UTF-16 の位置・重なりはまとめる・書記素の途中では切らない)。 */
    fun ranges(text: String, words: List<String>): List<IntRange> {
        if (text.isEmpty() || words.isEmpty()) return emptyList()
        // 元の書記素ごとに畳んだコードポイントを並べ、コードポイント → 元の書記素の番号を覚える
        val bounds = text.graphemeBounds()
        val hay = mutableListOf<Int>()
        val owner = mutableListOf<Int>()
        for ((gi, r) in bounds.withIndex()) {
            fold(text.substring(r.first, r.last + 1)).codePoints().forEach {
                hay.add(it)
                owner.add(gi)
            }
        }
        val spans = mutableListOf<Pair<Int, Int>>() // 書記素の [lo, hi)
        for (w in words) {
            val needle = fold(w).codePoints().toArray()
            if (needle.isEmpty() || needle.size > hay.size) continue
            var i = 0
            while (i + needle.size <= hay.size) {
                if ((needle.indices).all { hay[i + it] == needle[it] }) {
                    spans.add(owner[i] to owner[i + needle.size - 1] + 1)
                    i += needle.size
                } else {
                    i += 1
                }
            }
        }
        spans.sortBy { it.first }
        val merged = mutableListOf<Pair<Int, Int>>()
        for (s in spans) {
            val last = merged.lastOrNull()
            if (last != null && s.first <= last.second) merged[merged.size - 1] = last.first to maxOf(last.second, s.second)
            else merged.add(s)
        }
        return merged.map { (lo, hi) -> bounds[lo].first until (if (hi < bounds.size) bounds[hi].first else text.length) }
    }
}
