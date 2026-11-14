package jp.kz2601.metamon.services.mock

import jp.kz2601.metamon.services.AppUsage
import jp.kz2601.metamon.services.MyUsage
import jp.kz2601.metamon.services.PlanUsageSource
import jp.kz2601.metamon.services.UsageMeter

/**
 * サーバー無しの使用量(iOS の `DebugMockUsage`)。つまみ `METAMON_MOCK_USAGE` の書き方:
 * `apps:9/10,records:4200/5000,photos:10/500[,pro][,participant]`。壊れた項目は無視する。
 */
class MockPlanUsageSource(spec: String) : PlanUsageSource {
    val meters: Map<String, UsageMeter>
    val pro: Boolean
    val participant: Boolean

    init {
        val parts = spec.split(",").map(String::trim).filter(String::isNotEmpty)
        pro = "pro" in parts
        participant = "participant" in parts
        meters = parts.mapNotNull { part ->
            val kv = part.split(":")
            val nums = kv.getOrNull(1)?.split("/")?.mapNotNull { it.trim().toIntOrNull() }
            if (kv.size == 2 && nums?.size == 2) kv[0] to UsageMeter(used = nums[0], limit = nums[1]) else null
        }.toMap()
    }

    private val plan get() = if (pro) "pro" else "free"

    override suspend fun myUsage(): MyUsage = MyUsage(plan = plan, apps = meters["apps"])

    override suspend fun appUsage(appId: String): AppUsage = AppUsage(
        appId = appId, plan = plan, payer = if (participant) "creator" else "self", upgrade = !pro,
        records = meters["records"], photos = meters["photos"],
    )
}
