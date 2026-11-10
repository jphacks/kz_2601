package jp.kz2601.metamon.services.mock

import jp.kz2601.metamon.services.AppSummary
import jp.kz2601.metamon.services.Record
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import java.time.Instant
import java.time.LocalDate
import java.time.ZoneId

/** モックの初期データ(iOS の `MockSeed`)。entity id は docs/dsl/examples に合わせる。 */
object MockSeed {
    fun demoRecords(now: Instant = Instant.now(), zone: ZoneId = ZoneId.systemDefault()): Map<String, List<Record>> {
        val today = LocalDate.ofInstant(now, zone)
        val d = today.toString()
        val iso = now.toString()
        fun r(id: String, vararg data: Pair<String, Any>) = Record(id = id, data = obj(*data), createdAt = iso)
        return mapOf(
            "expense" to listOf(
                r("1", "amount" to 980, "category" to "食費", "memo" to "ランチ", "date" to d),
                r("2", "amount" to 420, "category" to "交通", "memo" to "電車", "date" to d),
            ),
            "workout" to listOf(r("1", "exercise" to "スクワット", "reps" to 20, "weight" to 40, "done" to true, "date" to iso)),
            "memo" to listOf(r("1", "note" to "きれいな紅葉", "date" to iso)),
            "groceries" to listOf(
                r("g1", "name" to "牛乳", "category" to "食品", "qty" to 1, "done" to false, "added" to d),
                r("g2", "name" to "卵", "category" to "食品", "qty" to 1, "done" to false, "added" to d),
                r("g3", "name" to "トイレットペーパー", "category" to "日用品", "qty" to 2, "done" to false, "added" to d),
                r("g4", "name" to "パン", "category" to "食品", "qty" to 1, "done" to true, "added" to d),
            ),
            "medicine" to listOf(
                r(
                    "d1", "name" to "血圧の薬", "timing" to "朝", "taken_on" to d,
                    // 毎日のチェックの履歴(iOS `DailyCheck.historyKey` = 欄名 + "__dates")
                    "taken_on__dates" to (2 downTo 0).map { today.minusDays(it.toLong()).toString() },
                ),
                r("d2", "name" to "ビタミン剤", "timing" to "昼", "taken_on" to today.minusDays(1).toString()),
                r("d3", "name" to "胃薬", "timing" to "夜"),
            ),
            // warikan(#101): 合計 ¥15,000 → 1 人 ¥5,000
            "members" to listOf(r("m1", "name" to "たろう"), r("m2", "name" to "はなこ"), r("m3", "name" to "じろう")),
            "expenses" to listOf(
                r("e1", "title" to "宿", "amount" to 9000, "payer" to "m1", "date" to d),
                r("e2", "title" to "夕食", "amount" to 4500, "payer" to "m2", "date" to d),
                r("e3", "title" to "ガソリン", "amount" to 1500, "payer" to "m1", "date" to d),
            ),
        )
    }

    /** 変身一覧に出す 2 件(iOS `DebugMockSharing.apps`)。 */
    val sharingApps: List<AppSummary> = listOf(
        AppSummary(id = "uitest-kakeibo", name = "家計簿", currentVersion = 3, updatedAt = "2026-10-24T09:00:00Z"),
        AppSummary(id = "uitest-shopping", name = "買い物リスト", currentVersion = 2, updatedAt = "2026-10-23T09:00:00Z"),
    )

    internal fun obj(vararg pairs: Pair<String, Any>): JsonObject = JsonObject(pairs.associate { (k, v) -> k to element(v) })

    private fun element(v: Any): JsonElement = when (v) {
        is JsonElement -> v
        is String -> JsonPrimitive(v)
        is Number -> JsonPrimitive(v)
        is Boolean -> JsonPrimitive(v)
        is List<*> -> JsonArray(v.map { element(it!!) })
        else -> JsonPrimitive(v.toString())
    }
}
