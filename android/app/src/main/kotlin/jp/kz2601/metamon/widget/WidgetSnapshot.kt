package jp.kz2601.metamon.widget

import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.buildJsonArray
import kotlinx.serialization.json.buildJsonObject
import kotlinx.serialization.json.contentOrNull
import kotlinx.serialization.json.doubleOrNull
import kotlinx.serialization.json.jsonPrimitive
import kotlinx.serialization.json.longOrNull
import kotlinx.serialization.json.put

/**
 * 本体 → ホーム画面のウィジェットの受け渡し(iOS の `WidgetSnapshot`・#912)。集計は本体で済ませ、ウィジェットは文字を描くだけ。
 * [appId] は押した時に開くアプリと、アプリを消した時に消すかの判定に使う。
 */
data class WidgetSnapshot(
    val appId: String,
    val appName: String,
    val stats: List<Stat>,
    val updatedAtMillis: Long,
    /** 以下は大きいサイズ用。「最近の記録」の entity 名。 */
    val recentTitle: String? = null,
    /** 新しい順の記録。null = 出さない / 空 = まだ記録が無い。 */
    val recent: List<Row>? = null,
    val trend: Trend? = null,
) {
    /** entity|aggregate|field|label。DSL を変えても同じ stat なら前回値を引き継ぐ。value が null = 記録をまだ読めていない。 */
    data class Stat(val key: String, val label: String, val value: String?)

    data class Row(val id: String, val title: String, val detail: String?, val value: String?)

    /** 直近 7 日の棒グラフ。[Point.epochDay] は端末のタイムゾーンの日。 */
    data class Trend(val metric: String?, val total: String, val points: List<Point>) {
        data class Point(val epochDay: Long, val value: Double)
    }

    /** 更新時刻以外が同じか(同じなら書き込み・再描画を省く)。 */
    fun hasSameContent(other: WidgetSnapshot?): Boolean = other != null && copy(updatedAtMillis = other.updatedAtMillis) == other

    fun encode(): String = Json.encodeToString(JsonElement.serializer(), toJson())

    private fun toJson(): JsonObject = buildJsonObject {
        put("appId", appId)
        put("appName", appName)
        put("updatedAt", updatedAtMillis)
        put("stats", buildJsonArray { stats.forEach { s -> add(buildJsonObject { put("key", s.key); put("label", s.label); put("value", s.value) }) } })
        recentTitle?.let { put("recentTitle", it) }
        recent?.let { rows ->
            put("recent", buildJsonArray {
                rows.forEach { r -> add(buildJsonObject { put("id", r.id); put("title", r.title); put("detail", r.detail); put("value", r.value) }) }
            })
        }
        trend?.let { t ->
            put("trend", buildJsonObject {
                put("metric", t.metric)
                put("total", t.total)
                put("points", buildJsonArray { t.points.forEach { p -> add(buildJsonObject { put("day", p.epochDay); put("value", p.value) }) } })
            })
        }
    }

    companion object {
        /** 壊れていれば null(ウィジェットは「アプリを作るとここに出ます」)。 */
        fun decode(text: String?): WidgetSnapshot? = try {
            text?.let { fromJson(Json.parseToJsonElement(it) as JsonObject) }
        } catch (_: Exception) {
            null
        }

        private fun fromJson(o: JsonObject): WidgetSnapshot = WidgetSnapshot(
            appId = o.str("appId")!!,
            appName = o.str("appName").orEmpty(),
            updatedAtMillis = o["updatedAt"]?.jsonPrimitive?.longOrNull ?: 0,
            stats = o.objects("stats").map { Stat(it.str("key").orEmpty(), it.str("label").orEmpty(), it.str("value")) },
            recentTitle = o.str("recentTitle"),
            recent = (o["recent"] as? JsonArray)?.let { o.objects("recent").map { Row(it.str("id").orEmpty(), it.str("title").orEmpty(), it.str("detail"), it.str("value")) } },
            trend = (o["trend"] as? JsonObject)?.let { t ->
                Trend(
                    t.str("metric"),
                    t.str("total").orEmpty(),
                    t.objects("points").map { Trend.Point(it["day"]?.jsonPrimitive?.longOrNull ?: 0, it["value"]?.jsonPrimitive?.doubleOrNull ?: 0.0) },
                )
            },
        )

        private fun JsonObject.str(key: String): String? = (this[key] as? JsonPrimitive)?.takeUnless { it is JsonNull }?.contentOrNull

        private fun JsonObject.objects(key: String): List<JsonObject> = (this[key] as? JsonArray)?.filterIsInstance<JsonObject>().orEmpty()
    }
}
