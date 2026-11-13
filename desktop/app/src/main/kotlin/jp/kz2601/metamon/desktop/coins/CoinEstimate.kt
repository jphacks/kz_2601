package jp.kz2601.metamon.desktop.coins

import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.layout.wrapContentWidth
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.clearAndSetSemantics
import androidx.compose.ui.semantics.contentDescription
import androidx.compose.ui.unit.dp
import jp.kz2601.metamon.desktop.i18n.Strings
import jp.kz2601.metamon.services.http.ApiJson
import jp.kz2601.metamon.services.http.HttpRequest
import jp.kz2601.metamon.services.http.MetamonHttp
import kotlinx.coroutines.CancellationException
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.booleanOrNull
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.intOrNull
import kotlinx.serialization.json.jsonObject

/** 処理の種類(`estimates` のキー・iOS の `CoinOperation`)。古い名前も受ける。 */
enum class CoinOperation(val keys: List<String>) {
    /** 新しく作る */
    GENERATE(listOf("generate", "create")),

    /** 直す */
    EDIT(listOf("edit")),

    /** 作る前の相談 */
    CHAT(listOf("chat")),

    /** アプリの中の AI */
    TEXT(listOf("text", "aibutton", "aichat")),
}

/**
 * 処理ごとの「約 N コイン」(#3363・`GET /v1/coins/config` の `estimates`・iOS の `CoinEstimates`)。
 * 数はすべてサーバーから。アプリに値段・枚数を持たない。コイン課金が `enforce` でない時・読めない時は何も出さない。
 */
class CoinEstimates(values: Map<String, Int> = emptyMap()) {
    private val values: Map<String, Int> = values.entries.associate { normalize(it.key) to it.value }

    /** 無い・0 以下は null(出さない) */
    fun coins(op: CoinOperation): Int? = op.keys.firstNotNullOfOrNull { k -> values[normalize(k)]?.takeIf { it > 0 } }

    companion object {
        val NONE = CoinEstimates()

        private fun normalize(key: String) = key.lowercase().replace("_", "").replace("-", "")

        /**
         * `CoinConfig` の本文から読む(寛容に: 無い・型が違う項目は飛ばす)。`mode` が `enforce`(または `enabled: true`)の時だけ目安を持つ。
         * `{ "generate": 30 }` と `{ "generate": { "coins": 30 } }` の両方を受ける。
         */
        fun fromConfig(body: String): CoinEstimates {
            val root = runCatching { ApiJson.parseToJsonElement(body).jsonObject }.getOrNull() ?: return NONE
            val enabled = (root["enabled"] as? JsonPrimitive)?.booleanOrNull
                ?: ((root["mode"] as? JsonPrimitive)?.contentOrNull?.lowercase() == "enforce")
            if (!enabled) return NONE
            val raw = root["estimates"] as? JsonObject ?: return NONE
            return CoinEstimates(raw.mapNotNull { (k, v) -> int(v)?.let { k to it } }.toMap())
        }

        private fun int(v: JsonElement?): Int? = when (v) {
            is JsonPrimitive -> v.intOrNull
            is JsonObject -> int(v["coins"]) ?: int(v["median"])
            else -> null
        }
    }
}

/** 目安を読む口。null = サーバーに無い・読めない(出さない)。 */
fun interface CoinEstimateSource {
    suspend fun estimates(): CoinEstimates?
}

/** `GET /v1/coins/config`(認証不要)。失敗は出さない扱い。 */
class HttpCoinEstimateSource(private val http: MetamonHttp) : CoinEstimateSource {
    override suspend fun estimates(): CoinEstimates? = runCatching {
        CoinEstimates.fromConfig(http.send(HttpRequest("GET", listOf("v1", "coins", "config"), timeoutMillis = 10_000L)))
    }.getOrElse { if (it is CancellationException) throw it else null }
}

/** 入力欄の下に出す文と読み上げ。出さない時は null。 */
data class CoinEstimateLabel(val text: String, val a11y: String) {
    companion object {
        fun of(estimates: CoinEstimates?, op: CoinOperation): CoinEstimateLabel? {
            val n = estimates?.coins(op) ?: return null
            return CoinEstimateLabel(Strings.format("desktop_coin_estimate", n), Strings.format("desktop_coin_estimate_a11y", n))
        }
    }
}

/** 作る・直すの入力欄の下の「約 N コイン」(iOS の `CoinEstimateLine`)。目安が無ければ何も描かない。 */
@Composable
fun CoinEstimateLine(estimates: CoinEstimates?, op: CoinOperation, modifier: Modifier = Modifier) {
    val label = CoinEstimateLabel.of(estimates, op) ?: return
    Text(
        label.text,
        style = MaterialTheme.typography.labelSmall,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
        modifier = modifier.fillMaxWidth().wrapContentWidth(Alignment.End).padding(top = 2.dp)
            .clearAndSetSemantics { contentDescription = label.a11y },
    )
}
