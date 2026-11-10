package jp.kz2601.metamon.dsl.logic

import java.text.BreakIterator

// Swift の String の数え方(Character = 書記素・unicodeScalars = コードポイント)と空白の扱いを Kotlin でそろえる小道具。

/** Swift の `Character` の数(書記素クラスタ)。 */
internal fun String.graphemeCount(): Int {
    val it = BreakIterator.getCharacterInstance()
    it.setText(this)
    var n = 0
    while (it.next() != BreakIterator.DONE) n++
    return n
}

/** 先頭から書記素 n 個(Swift の `prefix(n)`)。 */
internal fun String.graphemePrefix(n: Int): String {
    if (n <= 0) return ""
    val it = BreakIterator.getCharacterInstance()
    it.setText(this)
    var end = 0
    for (i in 0 until n) {
        val next = it.next()
        if (next == BreakIterator.DONE) return this
        end = next
    }
    return substring(0, end)
}

/** 書記素ごとの [start, end)(UTF-16 の位置)。 */
internal fun String.graphemeBounds(): List<IntRange> {
    val it = BreakIterator.getCharacterInstance()
    it.setText(this)
    val out = mutableListOf<IntRange>()
    var start = it.first()
    var end = it.next()
    while (end != BreakIterator.DONE) {
        out.add(start until end)
        start = end
        end = it.next()
    }
    return out
}

internal fun String.scalarCount(): Int = codePointCount(0, length)

/** Swift の `.whitespaces`(空白・タブ。改行は含まない)。 */
internal fun isSwiftSpace(c: Char): Boolean = c == '\t' || Character.isSpaceChar(c)

/** Swift の `.whitespacesAndNewlines`。 */
internal fun isSwiftWhitespace(c: Char): Boolean =
    isSwiftSpace(c) || c == '\n' || c == '\r' || c == '\u000B' || c == '\u000C' || c == '\u0085' || c == ' ' || c == ' '

internal fun String.trimSpaces(): String = trim(::isSwiftSpace)

internal fun String.trimWhitespace(): String = trim(::isSwiftWhitespace)
