package jp.kz2601.metamon.dsl.expr

import java.math.BigDecimal
import java.math.MathContext
import java.math.RoundingMode
import java.text.BreakIterator
import java.util.Locale
import kotlin.math.abs
import kotlin.math.sign
import kotlin.math.truncate

/**
 * iOS(Swift / Foundation)と JVM で違う数の扱いを iOS に合わせる。backend の expreval(doc.go)と同じ方針。
 * - 文字列 → 数値は Swift の `Double(String)`(前後の空白なし・"inf"/"nan" 可・16 進は p 無しでも可)
 * - 数値 → 文字列は Swift の `String(Double)`("1e+16"・"0.1"・"1000000000000000.0")
 * - `.rounded()` は 0.5 を 0 から遠い方へ
 * - `pow` は最も近い Double に丸める(Foundation.pow と同じ)
 * - 文字数は書記素クラスタ(Swift の `String.count`)
 */
object SwiftNumbers {
    private val decimal = Regex("""^[+-]?([0-9]+\.?[0-9]*|\.[0-9]+)([eE][+-]?[0-9]+)?$""")
    private val hex = Regex("""^[+-]?0[xX]([0-9a-fA-F]+\.?[0-9a-fA-F]*|\.[0-9a-fA-F]+)([pP][+-]?[0-9]+)?$""")
    private val special = Regex("""^([+-]?)(inf|infinity|nan)$""", RegexOption.IGNORE_CASE)

    /** Swift の `Double(String)`。読めなければ null。桁あふれは ±inf。 */
    fun parseDouble(s: String): Double? {
        if (s.isEmpty()) return null
        special.matchEntire(s)?.let { m ->
            val neg = m.groupValues[1] == "-"
            return if (m.groupValues[2].lowercase() == "nan") Double.NaN
            else if (neg) Double.NEGATIVE_INFINITY else Double.POSITIVE_INFINITY
        }
        if (decimal.matches(s)) return s.toDouble()
        if (hex.matches(s)) {
            val withExp = if (s.contains('p') || s.contains('P')) s else s + "p0"
            return java.lang.Double.parseDouble(withExp)
        }
        return null
    }

    /** Swift の `String(Double)`(description)。 */
    fun description(n: Double): String {
        if (n.isNaN()) return "nan"
        if (n.isInfinite()) return if (n > 0) "inf" else "-inf"
        if (n == 0.0) return if (1.0 / n < 0) "-0.0" else "0.0"
        val a = abs(n)
        val bd = shortest(n)
        if (a < 1e-4 || a > 9007199254740992.0) {
            val digits = bd.unscaledValue().abs().toString()
            val exp = digits.length - 1 - bd.scale()
            val mant = if (digits.length > 1) digits[0] + "." + digits.substring(1) else digits
            val e = abs(exp).toString().padStart(2, '0')
            return (if (n < 0) "-" else "") + mant + "e" + (if (exp < 0) "-" else "+") + e
        }
        val s = bd.toPlainString()
        return if (s.contains('.')) s else "$s.0"
    }

    /** 元の Double に戻る最短の 10 進(Swift の description と同じ桁。Double.toString は 4.9E-324 など違う時がある)。 */
    private fun shortest(n: Double): BigDecimal {
        val exact = BigDecimal(n)
        for (p in 1..17) {
            val r = exact.round(MathContext(p, RoundingMode.HALF_EVEN))
            if (r.toDouble() == n) return r.stripTrailingZeros()
        }
        return exact.stripTrailingZeros()
    }

    /** Swift の `.rounded()`(toNearestOrAwayFromZero)。-0.3 → -0.0。 */
    fun rounded(x: Double): Double {
        if (!x.isFinite()) return x
        val t = truncate(x)
        return if (abs(x - t) >= 0.5) t + sign(x) else t
    }

    /** 書記素クラスタの数(Swift の `String.count`)。 */
    fun graphemeCount(s: String): Int {
        if (s.isEmpty()) return 0
        val it = BreakIterator.getCharacterInstance(Locale.ROOT)
        it.setText(s)
        var n = 0
        while (it.next() != BreakIterator.DONE) n++
        return n
    }

    // MARK: - pow(最も近い Double に丸める)

    private val mc = MathContext(80, RoundingMode.HALF_EVEN)
    private val ln2: BigDecimal by lazy { lnMantissa(BigDecimal("0.5")).negate() }

    /**
     * Foundation.pow と同じく正しく丸めた pow。Math.pow は最後の 1 ビットがずれることがあるので、
     * 高精度の対数で隣の Double と比べて最も近い方へ寄せる(backend の pow.go と同じ方法)。
     */
    fun pow(a: Double, b: Double): Double {
        val y = Math.pow(a, b)
        if (y == 0.0 || y.isInfinite() || y.isNaN() || abs(y) < java.lang.Double.MIN_NORMAL ||
            a == 0.0 || a.isInfinite() || b.isInfinite() || b == 0.0 || b == 1.0
        ) return y
        val t = ln(BigDecimal(abs(a))).multiply(BigDecimal(b), mc)
        var m = abs(y)
        fun below(p: Double, q: Double): Boolean {
            val mid = BigDecimal(p).add(BigDecimal(q)).divide(BigDecimal(2))
            return t < ln(mid)
        }
        repeat(4) {
            val up = Math.nextUp(m)
            if (!up.isInfinite() && !below(m, up)) {
                m = up
                return@repeat
            }
            val down = Math.nextDown(m)
            if (down >= java.lang.Double.MIN_NORMAL && below(down, m)) {
                m = down
                return@repeat
            }
            return Math.copySign(m, y)
        }
        return Math.copySign(m, y)
    }

    /** ln(x)(x > 0)。x = m·2^e(m は [0.5, 1) 付近)に分けて級数で。 */
    private fun ln(x: BigDecimal): BigDecimal {
        val e = Math.getExponent(x.toDouble()) + 1
        val m = (if (e >= 0) x.multiply(BigDecimal("0.5").pow(e)) else x.multiply(BigDecimal(2).pow(-e))).round(mc)
        return lnMantissa(m).add(ln2.multiply(BigDecimal(e), mc), mc)
    }

    /** ln(m) = 2·atanh((m-1)/(m+1))。 */
    private fun lnMantissa(m: BigDecimal): BigDecimal {
        val one = BigDecimal.ONE
        var z = m.subtract(one, mc)
        if (z.signum() == 0) return z
        z = z.divide(m.add(one, mc), mc)
        val z2 = z.multiply(z, mc)
        var sum = z
        var pw = z
        var n = 3L
        val eps = BigDecimal.ONE.movePointLeft(90)
        while (true) {
            pw = pw.multiply(z2, mc)
            val term = pw.divide(BigDecimal(n), mc)
            sum = sum.add(term, mc)
            if (term.abs() < eps) break
            n += 2
        }
        return sum.multiply(BigDecimal(2), mc)
    }
}
