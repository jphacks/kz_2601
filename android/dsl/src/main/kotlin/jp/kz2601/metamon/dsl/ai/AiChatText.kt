package jp.kz2601.metamon.dsl.ai

import kotlinx.serialization.Serializable

// aiChat(task chat)が `POST /v1/text` に足す値(iOS の MetamonDSL `AIText.swift`・`AIChatSaveTo.swift`)。

/** `messages` の 1 発言(role は user | assistant)。 */
@Serializable
data class AiTextMessage(val role: String, val text: String)

/** aiChat saveTo の field の形(task chat だけ)。付けると返事が記録の案(`AiTextResult.suggest`)を返せる。 */
@Serializable
data class AiTextSaveTo(
    val entity: String,
    val name: String? = null,
    /** 端末の今日(YYYY-MM-DD)。「今日」「昨日」の日付に使う */
    val today: String,
    val fields: List<AiTextSaveField>,
)

@Serializable
data class AiTextSaveField(
    val id: String,
    val name: String,
    /** string | text | number | date | enum */
    val type: String,
    val options: List<String>? = null,
    val unit: String? = null,
    /** required で defaultValue の無い field(null = false) */
    val required: Boolean? = null,
)

/** aiChat inputEntity の「参考の記録」(利用者自身の記録・8 KB まで)。サーバーは最初の発言に `<records>` で入れる。 */
@Serializable
data class AiTextMaterial(val entity: String, val name: String? = null, val text: String)

/** UTF-8 のバイト数。 */
internal fun String.utf8Size(): Int = toByteArray(Charsets.UTF_8).size
