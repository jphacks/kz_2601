package jp.kz2601.metamon.renderer.components

/**
 * `markdown` のインライン記法(iOS の `AttributedString(markdown:, .inlineOnlyPreservingWhitespace)` と同じ範囲)。
 * `**太字**` `__太字__` `*斜体*` `_斜体_` `~~取り消し~~` `` `code` `` `[文字](url)` `<https://…>` と `\` のエスケープ。
 * 見出し・箇条書きなどブロックの記法は解釈せず文字のまま、改行・空白もそのまま残す。閉じない記号は文字のまま。
 */
object InlineMarkdown {
    data class Span(
        val text: String,
        val bold: Boolean = false,
        val italic: Boolean = false,
        val strike: Boolean = false,
        val code: Boolean = false,
        val url: String? = null,
    )

    private data class Marks(val bold: Boolean = false, val italic: Boolean = false, val strike: Boolean = false, val url: String? = null)

    private const val PUNCT = "!\"#$%&'()*+,-./:;<=>?@[\\]^_`{|}~"

    fun parse(src: String): List<Span> {
        val out = mutableListOf<Span>()
        parseInto(src, Marks(), out)
        // 同じ印の隣どうしをまとめる
        val merged = mutableListOf<Span>()
        for (s in out) {
            val last = merged.lastOrNull()
            if (last != null && last.copy(text = "") == s.copy(text = "")) merged[merged.size - 1] = last.copy(text = last.text + s.text)
            else if (s.text.isNotEmpty()) merged += s
        }
        return merged
    }

    /** 記号を外した文(読み上げ・テスト用)。 */
    fun plain(src: String): String = parse(src).joinToString("") { it.text }

    private fun parseInto(s: String, m: Marks, out: MutableList<Span>) {
        val buf = StringBuilder()
        fun flush() {
            if (buf.isNotEmpty()) out += Span(buf.toString(), m.bold, m.italic, m.strike, url = m.url)
            buf.clear()
        }
        var i = 0
        while (i < s.length) {
            val ch = s[i]
            // エスケープ
            if (ch == '\\' && i + 1 < s.length && s[i + 1] in PUNCT) {
                buf.append(s[i + 1]); i += 2; continue
            }
            // code(同じ長さの ` で閉じる)
            if (ch == '`') {
                val run = s.runLength(i, '`')
                val close = s.findRun(i + run, '`', run)
                if (close >= 0) {
                    flush()
                    var body = s.substring(i + run, close)
                    if (body.length >= 2 && body.startsWith(' ') && body.endsWith(' ') && body.isNotBlank()) body = body.substring(1, body.length - 1)
                    out += Span(body, m.bold, m.italic, m.strike, code = true, url = m.url)
                    i = close + run; continue
                }
                buf.append(s, i, i + run); i += run; continue
            }
            // 自動リンク <https://…>
            if (ch == '<' && m.url == null) {
                val close = s.indexOf('>', i + 1)
                val inner = if (close > 0) s.substring(i + 1, close) else ""
                if (close > 0 && inner.isNotEmpty() && inner.none { it.isWhitespace() || it == '<' } && SCHEME.matches(inner)) {
                    flush()
                    out += Span(inner, m.bold, m.italic, m.strike, url = inner)
                    i = close + 1; continue
                }
            }
            // リンク [文字](url)
            if (ch == '[' && m.url == null) {
                val closeBracket = s.matchingBracket(i)
                if (closeBracket > 0 && closeBracket + 1 < s.length && s[closeBracket + 1] == '(') {
                    val closeParen = s.indexOf(')', closeBracket + 2)
                    if (closeParen > 0) {
                        val dest = s.substring(closeBracket + 2, closeParen).trim().removeSurrounding("<", ">")
                        if (dest.none { it.isWhitespace() }) {
                            flush()
                            parseInto(s.substring(i + 1, closeBracket), m.copy(url = dest), out)
                            i = closeParen + 1; continue
                        }
                    }
                }
            }
            // 強調・取り消し
            val delim = when {
                s.startsWith("**", i) -> "**"
                s.startsWith("__", i) -> "__"
                s.startsWith("~~", i) -> "~~"
                ch == '*' || ch == '_' -> ch.toString()
                else -> null
            }
            if (delim != null && canOpen(s, i, delim)) {
                val close = findCloser(s, i + delim.length, delim)
                if (close > 0) {
                    flush()
                    val inner = s.substring(i + delim.length, close)
                    val next = when (delim) {
                        "**", "__" -> m.copy(bold = true)
                        "~~" -> m.copy(strike = true)
                        else -> m.copy(italic = true)
                    }
                    parseInto(inner, next, out)
                    i = close + delim.length; continue
                }
            }
            buf.append(ch); i++
        }
        flush()
    }

    private val SCHEME = Regex("^[A-Za-z][A-Za-z0-9+.-]{1,31}:.+")

    /** 開く記号: 直後が空白でない。`_` は語の途中(snake_case)では開かない。 */
    private fun canOpen(s: String, i: Int, d: String): Boolean {
        val after = s.getOrNull(i + d.length) ?: return false
        if (after.isWhitespace()) return false
        if (d[0] == '_' && i > 0 && s[i - 1].isLetterOrDigit()) return false
        return true
    }

    /** 閉じる記号: 直前が空白でない。1 文字の * / _ は ** / __ の一部を閉じに使わない。 */
    private fun findCloser(s: String, from: Int, d: String): Int {
        var j = from
        while (j < s.length) {
            if (s[j] == '\\') { j += 2; continue }
            if (s[j] == '`') {
                val run = s.runLength(j, '`')
                val close = s.findRun(j + run, '`', run)
                j = if (close >= 0) close + run else j + run
                continue
            }
            if (s[j] == d[0] && (d.length == 1 || d[1] == d[0])) {
                val run = s.runLength(j, d[0])
                // 並びの最後の d で閉じる(`**a *b***` は b を斜体で閉じてから太字を閉じる)。1 文字の d は単独の記号だけ
                val at = if (d.length == 2 && run >= 2) j + run - 2 else if (d.length == 1 && run == 1) j else -1
                val intraword = d[0] == '_' && s.getOrNull(at + d.length)?.isLetterOrDigit() == true
                if (at > from && !s[at - 1].isWhitespace() && !intraword) return at
                j += run
                continue
            }
            j++
        }
        return -1
    }

    private fun String.runLength(i: Int, c: Char): Int {
        var n = 0
        while (i + n < length && this[i + n] == c) n++
        return n
    }

    /** 長さ `n` ちょうどの `c` の並びの位置。 */
    private fun String.findRun(from: Int, c: Char, n: Int): Int {
        var j = from
        while (j < length) {
            if (this[j] == c) {
                val run = runLength(j, c)
                if (run == n) return j
                j += run
            } else {
                j++
            }
        }
        return -1
    }

    private fun String.matchingBracket(open: Int): Int {
        var depth = 0
        var j = open
        while (j < length) {
            when (this[j]) {
                '\\' -> j++
                '[' -> depth++
                ']' -> if (--depth == 0) return j
            }
            j++
        }
        return -1
    }
}
