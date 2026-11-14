package jp.kz2601.metamon.dsl.logic

import java.text.BreakIterator
import java.util.Locale

// Swift の String の数え方(Character = 書記素・unicodeScalars = コードポイント)と空白の扱いを Kotlin でそろえる小道具。

/** Swift の `Character` の数(書記素クラスタ)。 */
internal fun String.graphemeCount(): Int = graphemeBreaks().size - 1

/** 先頭から書記素 n 個(Swift の `prefix(n)`)。 */
internal fun String.graphemePrefix(n: Int): String {
    if (n <= 0) return ""
    val b = graphemeBreaks()
    return if (n >= b.size - 1) this else substring(0, b[n])
}

/** 書記素ごとの [start, end)(UTF-16 の位置)。 */
internal fun String.graphemeBounds(): List<IntRange> {
    val b = graphemeBreaks()
    return (0 until b.size - 1).map { b[it] until b[it + 1] }
}

/**
 * 書記素の切れ目(0 と length を含む)。JDK 17 の BreakIterator は絵文字の ZWJ 連結・国旗・肌色を
 * 1 文字にまとめない(JDK 20 で拡張書記素クラスタに対応)ので、Unicode の GB9・GB11・GB12/13 で補ってそろえる。
 */
internal fun String.graphemeBreaks(): List<Int> {
    if (isEmpty()) return listOf(0)
    val it = BreakIterator.getCharacterInstance(Locale.ROOT)
    it.setText(this)
    val out = mutableListOf(0)
    var b = it.next()
    while (b != BreakIterator.DONE) {
        if (b == length || !joins(this, out.last(), b)) out.add(b)
        b = it.next()
    }
    return out
}

/** `start` から始まる書記素と `at` から始まる書記素をつなぐか。 */
private fun joins(s: String, start: Int, at: Int): Boolean {
    val prev = s.codePointBefore(at)
    val next = s.codePointAt(at)
    if (prev == 0x0D || prev == 0x0A || Character.getType(prev) == Character.CONTROL.toInt()) return false
    // GB9: 結合文字・ZWJ・異体字セレクタ・肌色・タグの前では切らない
    if (isExtend(next) || next == ZWJ) return true
    // GB11: 絵文字 Extend* ZWJ × 絵文字
    if (prev == ZWJ && isPictographic(next)) {
        var i = s.offsetByCodePoints(at, -1)
        while (i > start) {
            val c = s.codePointBefore(i)
            if (isPictographic(c)) return true
            if (!isExtend(c)) return false
            i -= Character.charCount(c)
        }
        return false
    }
    // GB12/13: 国旗(地域指示子)は 2 つずつ
    if (isRegional(prev) && isRegional(next)) {
        var n = 0
        var i = at
        while (i > 0 && isRegional(s.codePointBefore(i))) {
            n++
            i -= Character.charCount(s.codePointBefore(i))
        }
        return n % 2 == 1
    }
    return false
}

private const val ZWJ = 0x200D

private fun isRegional(c: Int) = c in 0x1F1E6..0x1F1FF

private fun isExtend(c: Int): Boolean = when {
    c in 0xFE00..0xFE0F || c in 0xE0100..0xE01EF -> true // 異体字セレクタ
    c in 0x1F3FB..0x1F3FF -> true // 肌色
    c in 0xE0020..0xE007F -> true // タグ(地域の旗)
    else -> when (Character.getType(c)) {
        Character.NON_SPACING_MARK.toInt(), Character.ENCLOSING_MARK.toInt() -> true
        else -> false
    }
}

/** Extended_Pictographic の近似(JDK 17 には Character.isExtendedPictographic が無い)。 */
private fun isPictographic(c: Int): Boolean = when (c) {
    0x00A9, 0x00AE, 0x203C, 0x2049, 0x2122, 0x2139, 0x24C2, 0x3030, 0x303D, 0x3297, 0x3299 -> true
    else -> c in 0x2194..0x21AA || c in 0x231A..0x23FF || c in 0x25AA..0x27BF || c in 0x2934..0x2935 ||
        c in 0x2B05..0x2B55 || (c in 0x1F000..0x1FAFF && !isRegional(c) && c !in 0x1F3FB..0x1F3FF) || c in 0x1FC00..0x1FFFD
}

internal fun String.scalarCount(): Int = codePointCount(0, length)

/** Swift の `.whitespaces`(空白・タブ。改行は含まない)。 */
internal fun isSwiftSpace(c: Char): Boolean = c == '\t' || Character.isSpaceChar(c)

/** Swift の `.whitespacesAndNewlines`。 */
internal fun isSwiftWhitespace(c: Char): Boolean =
    isSwiftSpace(c) || c == '\n' || c == '\r' || c == '\u000B' || c == '\u000C' || c == '\u0085' || c == ' ' || c == ' '

internal fun String.trimSpaces(): String = trim(::isSwiftSpace)

internal fun String.trimWhitespace(): String = trim(::isSwiftWhitespace)
