package jp.kz2601.metamon.dsl

import java.text.BreakIterator
import java.text.Normalizer

// `text.highlight` の語の決め方と照合(iOS の MetamonDSL/Model/TextHighlight.swift と同じ)。描画から切り離して JVM で試す。

/** 1 つの highlight で印をつける語の上限。 */
const val TEXT_HIGHLIGHT_MAX_WORDS = 20

/** 1 語の長さの上限(文字数)。 */
const val TEXT_HIGHLIGHT_MAX_WORD_LENGTH = 50

/** 印をつける語(空・重複を除き、先頭から 20 個)。`fieldValue` は field の今の値(「,」「、」改行で区切る)。 */
fun TextHighlight.resolvedWords(fieldValue: String?): List<String> {
    val raw = when {
        words.isNotEmpty() -> words
        field != null && fieldValue != null ->
            Normalizer.normalize(fieldValue, Normalizer.Form.NFKC).split(',', '、', '\n', '\r')
        else -> emptyList()
    }
    val seen = HashSet<String>()
    val out = mutableListOf<String>()
    for (w in raw) {
        val t = w.trim(' ', '\t', '　')
        if (t.isEmpty() || TextHighlightMatcher.length(t) > TEXT_HIGHLIGHT_MAX_WORD_LENGTH) continue
        if (!seen.add(TextHighlightMatcher.fold(t))) continue
        out += t
        if (out.size == TEXT_HIGHLIGHT_MAX_WORDS) break
    }
    return out
}

/** 語の照合。大文字小文字・全角半角を区別せず(NFKC + 小文字)、日本語は語の一部でも合う。 */
object TextHighlightMatcher {
    fun fold(s: String): String = Normalizer.normalize(s, Normalizer.Form.NFKC).lowercase()

    /** 見た目の 1 文字(書記素)の数。 */
    fun length(s: String): Int = graphemeStarts(s).size

    /**
     * `text` の中で `words` に合う範囲(UTF-16 の [start, end))。重なりはまとめ、見た目の 1 文字の途中では切らない。
     */
    fun ranges(text: String, words: List<String>): List<IntRange> {
        if (text.isEmpty() || words.isEmpty()) return emptyList()
        // 元の 1 文字ごとに畳んだコードポイントを並べ、どの文字から来たかを覚える
        val starts = graphemeStarts(text)
        val hay = mutableListOf<Int>()
        val owner = mutableListOf<Int>()
        starts.forEachIndexed { i, lo ->
            val hi = if (i + 1 < starts.size) starts[i + 1] else text.length
            fold(text.substring(lo, hi)).codePoints().forEach { hay += it; owner += i }
        }
        val spans = mutableListOf<Pair<Int, Int>>()
        for (w in words) {
            val needle = fold(w).codePoints().toArray()
            if (needle.isEmpty() || needle.size > hay.size) continue
            var i = 0
            while (i + needle.size <= hay.size) {
                if ((needle.indices).all { hay[i + it] == needle[it] }) {
                    spans += owner[i] to owner[i + needle.size - 1] + 1
                    i += needle.size
                } else {
                    i++
                }
            }
        }
        spans.sortBy { it.first }
        val merged = mutableListOf<Pair<Int, Int>>()
        for (s in spans) {
            val last = merged.lastOrNull()
            if (last != null && s.first <= last.second) merged[merged.size - 1] = last.first to maxOf(last.second, s.second)
            else merged += s
        }
        return merged.map { (lo, hi) -> starts[lo] until (if (hi < starts.size) starts[hi] else text.length) }
    }

    private fun graphemeStarts(s: String): List<Int> {
        val it = BreakIterator.getCharacterInstance().apply { setText(s) }
        val out = mutableListOf<Int>()
        var i = it.first()
        while (i != BreakIterator.DONE && i < s.length) { out += i; i = it.next() }
        return out
    }
}
