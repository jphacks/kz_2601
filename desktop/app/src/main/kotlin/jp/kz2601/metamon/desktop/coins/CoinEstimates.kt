package jp.kz2601.metamon.desktop.coins

import jp.kz2601.metamon.desktop.i18n.Strings
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlin.math.abs
import kotlin.math.roundToInt

/** 処理の種類(目安 `estimates` のキー)。iOS の `CoinOperation` と同じ。 */
enum class CoinOperation(internal val keys: List<String>) {
    /** 新しく作る */
    GENERATE(listOf("generate", "create")),

    /** 直す */
    EDIT(listOf("edit")),

    /** 作る前の相談 */
    CHAT(listOf("chat")),

    /** アプリの中の AI(aiButton・aiChat) */
    TEXT(listOf("text", "aibutton", "aichat")),
}

/**
 * 処理ごとの「約 N コイン」(`GET /v1/coins/config` の `estimates`)。iOS の `CoinEstimates` と同じ規則。
 * 数はすべてサーバーから。無い・0 以下は null(出さない)。
 */
class CoinEstimates(values: Map<String, Int> = emptyMap()) {
    private val values: Map<String, Int> =
        LinkedHashMap<String, Int>().also { out -> values.forEach { (k, v) -> out.putIfAbsent(normalize(k), v) } }

    val isEmpty: Boolean get() = values.isEmpty()

    fun coins(op: CoinOperation): Int? =
        op.keys.firstNotNullOfOrNull { values[normalize(it)]?.takeIf { n -> n > 0 } }

    override fun equals(other: Any?): Boolean = other is CoinEstimates && other.values == values

    override fun hashCode(): Int = values.hashCode()

    companion object {
        /** 大文字小文字・`_`・`-` は無視する。 */
        fun normalize(key: String): String = key.lowercase().replace("_", "").replace("-", "")

        /** `{"generate": 30}` と `{"generate": {"coins": 30}}`(`median` も)の両方を受ける。読めない項目は飛ばす。 */
        fun fromJson(json: JsonElement?): CoinEstimates {
            val out = LinkedHashMap<String, Int>()
            for ((k, v) in (json as? JsonObject).orEmpty()) {
                val obj = v as? JsonObject
                val n = int(v) ?: int(obj?.get("coins")) ?: int(obj?.get("median")) ?: continue
                out[k] = n
            }
            return CoinEstimates(out)
        }

        /** 数・数の文字を寛容に読む(iOS の `CoinJSON.int`)。真偽値・null・大きすぎる数は null。 */
        internal fun int(v: JsonElement?): Int? {
            val p = v as? JsonPrimitive ?: return null
            if (p.isString) return p.content.toIntOrNull()
            val d = p.content.toDoubleOrNull() ?: return null
            return if (d.isFinite() && abs(d) < 9e15) d.roundToInt() else null
        }
    }
}

/** コインの目安の文(iOS の `CoinCopy`)。描画はしない。 */
object CoinCopy {
    /** 押す前の目安。コインが無効・目安が無い時は null(何も出さない)。 */
    fun estimate(op: CoinOperation, estimates: CoinEstimates, enabled: Boolean): String? {
        if (!enabled) return null
        val n = estimates.coins(op) ?: return null
        return Strings.format("desktop_coin_estimate", n)
    }

    /** 読み上げ。残高が分かる時は残りも言う。 */
    fun estimateAccessibility(n: Int, balance: Int?): String =
        if (balance == null) {
            Strings.format("desktop_coin_estimate_a11y", n)
        } else {
            Strings.format("desktop_coin_estimate_a11y_balance", n, balance)
        }
}
